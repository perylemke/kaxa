package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// interactProtocol is the WebSocket subprotocol of the interaction stream.
	interactProtocol = "sandboxes.v1"
	handshakeTimeout = 30 * time.Second
	// maxFrame bounds one frame the server sends.
	maxFrame = 8 << 20
	// stdinChunk is the most input bytes sent in one frame.
	stdinChunk = 32 << 10

	defaultRows = 24
	defaultCols = 80
)

// ProcessSpec describes a command to run in a sandbox.
type ProcessSpec struct {
	Argv []string
	Dir  string
	Env  map[string]string
	User string
	TTY  bool
	Rows uint16
	Cols uint16
}

// Event is one thing a process did: wrote output or exited.
type Event struct {
	Data     []byte
	Stderr   bool
	Exited   bool
	ExitCode int
}

// Process is a running command with an interaction stream.
type Process interface {
	// Next blocks until the process writes or exits.
	Next() (Event, error)
	WriteStdin(p []byte) error
	CloseStdin() error
	Resize(rows, cols uint16) error
	Close() error
}

// processStarter starts commands in a sandbox through its endpoint.
type processStarter interface {
	Start(ctx context.Context, ep Endpoint, cred Credential, spec ProcessSpec) (Process, error)
}

// endpointClient is the processStarter that talks to a sandbox endpoint over
// HTTPS and WebSocket.
type endpointClient struct {
	http *http.Client
}

// checkEndpoint accepts HTTPS endpoints. Plain HTTP is allowed only for the
// loopback interface, which tests use; kaxa never sends a bearer token in the
// clear to another host.
func checkEndpoint(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, errors.New("docker: the sandbox endpoint is not a valid URL")
	}
	switch u.Scheme {
	case "https":
		return u, nil
	case "http":
		if ip := net.ParseIP(u.Hostname()); ip != nil && ip.IsLoopback() {
			return u, nil
		}
	}
	return nil, errors.New("docker: refusing a sandbox endpoint that is not HTTPS")
}

func endpointURL(u *url.URL, path string) string {
	c := *u
	c.Path = strings.TrimRight(c.Path, "/") + path
	c.RawQuery, c.Fragment = "", ""
	return c.String()
}

type createProcessBody struct {
	Cmd        []string          `json:"cmd"`
	Env        map[string]string `json:"env,omitempty"`
	WorkingDir string            `json:"workingDir,omitempty"`
	User       string            `json:"user,omitempty"`
	PTY        *ptyBody          `json:"pty,omitempty"`
}

type ptyBody struct {
	InitialSize terminalSize `json:"initialSize"`
}

type terminalSize struct {
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}

type processResponse struct {
	Name string `json:"name"`
}

// Start creates the process and attaches to its interaction stream.
func (c *endpointClient) Start(ctx context.Context, ep Endpoint, cred Credential, spec ProcessSpec) (Process, error) {
	base, err := checkEndpoint(ep.URI)
	if err != nil {
		return nil, err
	}
	body := createProcessBody{Cmd: spec.Argv, Env: spec.Env, WorkingDir: spec.Dir, User: spec.User}
	if spec.TTY {
		body.PTY = &ptyBody{InitialSize: sizeOrDefault(spec.Rows, spec.Cols)}
	}
	var created processResponse
	err = roundTrip(ctx, c.http, request{
		method: http.MethodPost,
		url:    endpointURL(base, "/v1/processes"),
		token:  cred.Token,
		in:     body,
		out:    &created,
	})
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(created.Name, "processes/") || resourceID(created.Name) == "" {
		return nil, errors.New("docker: the API returned no process name")
	}
	return dialInteract(ctx, base, created.Name, cred.Token)
}

func sizeOrDefault(rows, cols uint16) terminalSize {
	if rows == 0 || cols == 0 {
		return terminalSize{Rows: defaultRows, Cols: defaultCols}
	}
	return terminalSize{Rows: rows, Cols: cols}
}

func dialInteract(ctx context.Context, base *url.URL, process, token string) (Process, error) {
	ws := *base
	switch ws.Scheme {
	case "https":
		ws.Scheme = "wss"
	default:
		ws.Scheme = "ws"
	}
	target := endpointURL(&ws, "/v1/processes/"+url.PathEscape(resourceID(process))+"/interact")
	dialer := websocket.Dialer{
		HandshakeTimeout: handshakeTimeout,
		Subprotocols:     []string{interactProtocol},
		Proxy:            http.ProxyFromEnvironment,
	}
	header := http.Header{"Authorization": {"Bearer " + token}}
	conn, resp, err := dialer.DialContext(ctx, target, header) //nolint:bodyclose // closed below when the handshake fails
	if err != nil {
		if resp != nil {
			defer func() { _ = resp.Body.Close() }()
			return nil, &APIError{Status: resp.StatusCode, Code: codeForStatus(resp.StatusCode)}
		}
		return nil, fmt.Errorf("docker: attach to process: %w", urlErrorCause(err))
	}
	conn.SetReadLimit(maxFrame)
	p := &wsProcess{conn: conn}
	if err := p.write(map[string]any{"attach": map[string]string{"name": process}}); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return p, nil
}

// wsProcess is a Process over a WebSocket. gorilla/websocket allows one writer
// at a time, so writes take a lock; one goroutine reads.
type wsProcess struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (p *wsProcess) write(frame any) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.conn.WriteJSON(frame); err != nil {
		return fmt.Errorf("docker: write to process: %w", err)
	}
	return nil
}

// WriteStdin sends input in frames of at most stdinChunk bytes. The server
// reads "stdin" as base64, which encoding/json produces for []byte.
func (p *wsProcess) WriteStdin(data []byte) error {
	for len(data) > 0 {
		n := min(len(data), stdinChunk)
		if err := p.write(map[string][]byte{"stdin": data[:n]}); err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

func (p *wsProcess) CloseStdin() error {
	return p.write(map[string]bool{"closeStdin": true})
}

func (p *wsProcess) Resize(rows, cols uint16) error {
	return p.write(map[string]terminalSize{"resize": {Rows: rows, Cols: cols}})
}

func (p *wsProcess) Close() error {
	return p.conn.Close()
}

type frame struct {
	Chunk *struct {
		Stream string `json:"stream"`
		Data   []byte `json:"data"`
	} `json:"chunk"`
	Exited *struct {
		ExitCode int `json:"exitCode"`
	} `json:"exited"`
	Heartbeat *json.RawMessage `json:"heartbeat"`
	// Code and Message report a failed stream.
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Next returns the next output or the exit of the process. Heartbeats are
// skipped. A stream that ends before the exit is an error.
func (p *wsProcess) Next() (Event, error) {
	for {
		var f frame
		if err := p.conn.ReadJSON(&f); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				return Event{}, errors.New("docker: the process stream closed before the process exited")
			}
			return Event{}, fmt.Errorf("docker: read from process: %w", err)
		}
		switch {
		case f.Chunk != nil:
			return Event{Data: f.Chunk.Data, Stderr: f.Chunk.Stream == "stderr"}, nil
		case f.Exited != nil:
			return Event{Exited: true, ExitCode: f.Exited.ExitCode}, nil
		case f.Code != "":
			return Event{}, &APIError{Code: f.Code, Message: f.Message}
		}
	}
}
