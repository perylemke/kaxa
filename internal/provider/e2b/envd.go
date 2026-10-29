package e2b

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

const (
	// envdPort is where the daemon inside a sandbox listens.
	envdPort = 49983
	// maxFrameSize bounds one message of a stream, so a bad length cannot
	// make kaxa allocate without limit.
	maxFrameSize = 16 << 20
	// keepalivePing asks the daemon for a keepalive event every 50 seconds, so
	// a quiet command does not lose its stream.
	keepalivePing = "50"
	endStreamFlag = 0x02
	processSvc    = "/process.Process/"
)

// StartRequest describes one command to run in a sandbox.
type StartRequest struct {
	// Cmd is the executable and Args its arguments. They are never parsed by a shell.
	Cmd  string
	Args []string
	Env  map[string]string
	// Cwd is an absolute directory. Empty means the home directory of User.
	Cwd  string
	User string
	// PTY runs the command in a terminal of Rows x Cols.
	PTY  bool
	Rows uint16
	Cols uint16
	// Stdin attaches an input stream the command can read.
	Stdin bool
}

// EventKind tells what an Event carries.
type EventKind int

// Kinds of Event.
const (
	// EventStdout is output on standard output.
	EventStdout EventKind = iota + 1
	// EventStderr is output on standard error.
	EventStderr
	// EventPTY is terminal output, which merges both streams.
	EventPTY
	// EventEnd reports that the command finished.
	EventEnd
)

// Event is one thing a running command did.
type Event struct {
	Kind EventKind
	// Data is the output of a stdout, stderr or pty event.
	Data []byte
	// ExitCode and Err describe an end event. Err is set when the command
	// could not run or was stopped.
	ExitCode int
	Err      string
}

// Process is a command running in a sandbox.
type Process interface {
	// Next returns the next event and io.EOF once the end event was returned.
	Next() (Event, error)
	SendInput(ctx context.Context, data []byte) error
	// CloseStdin tells the command its input is over. It has no effect on a terminal.
	CloseStdin(ctx context.Context) error
	Resize(ctx context.Context, rows, cols uint16) error
	// Close drops the stream. The command keeps running in the sandbox.
	Close()
}

type envdClient interface {
	Start(ctx context.Context, conn Connection, req StartRequest) (Process, error)
	// Upload writes body to path, creating the parent directories.
	Upload(ctx context.Context, conn Connection, user, path string, body io.Reader) error
}

// httpEnvd talks to the daemon with the Connect protocol over HTTP and JSON.
type httpEnvd struct {
	http *http.Client
	// baseURL returns the daemon address of a sandbox.
	baseURL func(Connection) string
}

func newHTTPEnvd() *httpEnvd {
	return &httpEnvd{
		// No overall timeout: a stream lives as long as its command.
		http: &http.Client{},
		baseURL: func(c Connection) string {
			return "https://" + strconv.Itoa(envdPort) + "-" + c.ID + "." + c.Domain
		},
	}
}

func (e *httpEnvd) setHeaders(h http.Header, conn Connection, user string) {
	h.Set("X-Access-Token", conn.AccessToken)
	h.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":")))
}

type startBody struct {
	Process startProcess `json:"process"`
	PTY     *ptyBody     `json:"pty,omitempty"`
	Stdin   bool         `json:"stdin"`
}

type startProcess struct {
	Cmd  string            `json:"cmd"`
	Args []string          `json:"args,omitempty"`
	Envs map[string]string `json:"envs,omitempty"`
	Cwd  string            `json:"cwd,omitempty"`
}

type ptyBody struct {
	Size ptySize `json:"size"`
}

type ptySize struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

func (e *httpEnvd) Start(ctx context.Context, conn Connection, req StartRequest) (Process, error) {
	body := startBody{
		Process: startProcess{Cmd: req.Cmd, Args: req.Args, Envs: req.Env, Cwd: req.Cwd},
		Stdin:   req.Stdin,
	}
	if req.PTY {
		body.PTY = &ptyBody{Size: ptySize{Cols: req.Cols, Rows: req.Rows}}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		e.baseURL(conn)+processSvc+"Start", bytes.NewReader(envelope(payload)))
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/connect+json")
	httpReq.Header.Set("Connect-Protocol-Version", "1")
	httpReq.Header.Set("Keepalive-Ping-Interval", keepalivePing)
	e.setHeaders(httpReq.Header, conn, req.User)
	resp, err := e.http.Do(httpReq)
	if err != nil {
		cancel()
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		apiErr := apiErrorOf(resp)
		_ = resp.Body.Close()
		cancel()
		return nil, apiErr
	}
	p := &httpProcess{e: e, conn: conn, user: req.User, pty: req.PTY, body: resp.Body, r: bufio.NewReader(resp.Body), cancel: cancel}
	if err := p.awaitStart(); err != nil {
		p.Close()
		return nil, err
	}
	return p, nil
}

// envelope frames one message of a Connect stream: a flag byte, the length as
// four big-endian bytes, then the message.
func envelope(payload []byte) []byte {
	out := make([]byte, 5, 5+len(payload))
	binary.BigEndian.PutUint32(out[1:], uint32(len(payload))) // #nosec G115 -- a start request is a few kilobytes
	return append(out, payload...)
}

type httpProcess struct {
	e    *httpEnvd
	conn Connection
	user string
	pty  bool
	pid  uint32

	body   io.ReadCloser
	r      *bufio.Reader
	cancel context.CancelFunc
	done   bool

	closeOnce sync.Once
}

type wireEvent struct {
	Event struct {
		Start *struct {
			PID uint32 `json:"pid"`
		} `json:"start"`
		Data *struct {
			Stdout []byte `json:"stdout"`
			Stderr []byte `json:"stderr"`
			PTY    []byte `json:"pty"`
		} `json:"data"`
		End *struct {
			ExitCode int    `json:"exitCode"`
			Status   string `json:"status"`
			Error    string `json:"error"`
		} `json:"end"`
	} `json:"event"`
}

// awaitStart reads until the daemon reports the process ID, which later calls need.
func (p *httpProcess) awaitStart() error {
	for {
		ev, err := p.readWire()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return errors.New("e2b: the sandbox closed the stream before the command started")
			}
			return err
		}
		if ev.Event.Start != nil {
			p.pid = ev.Event.Start.PID
			return nil
		}
	}
}

func (p *httpProcess) Next() (Event, error) {
	if p.done {
		return Event{}, io.EOF
	}
	for {
		ev, err := p.readWire()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return Event{}, io.ErrUnexpectedEOF
			}
			return Event{}, err
		}
		if out, ok := p.eventOf(ev); ok {
			if out.Kind == EventEnd {
				p.done = true
			}
			return out, nil
		}
	}
}

func (p *httpProcess) eventOf(ev wireEvent) (Event, bool) {
	if d := ev.Event.Data; d != nil {
		switch {
		case len(d.Stdout) > 0:
			return Event{Kind: EventStdout, Data: d.Stdout}, true
		case len(d.Stderr) > 0:
			return Event{Kind: EventStderr, Data: d.Stderr}, true
		case len(d.PTY) > 0:
			return Event{Kind: EventPTY, Data: d.PTY}, true
		}
	}
	if end := ev.Event.End; end != nil {
		return Event{Kind: EventEnd, ExitCode: end.ExitCode, Err: end.Error}, true
	}
	return Event{}, false
}

// readWire reads one message. It returns io.EOF when the stream ends cleanly
// and the error the daemon sent when it ends with one.
func (p *httpProcess) readWire() (wireEvent, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(p.r, hdr[:]); err != nil {
		if errors.Is(err, io.EOF) {
			return wireEvent{}, io.EOF
		}
		return wireEvent{}, err
	}
	size := binary.BigEndian.Uint32(hdr[1:])
	if size > maxFrameSize {
		return wireEvent{}, fmt.Errorf("e2b: stream message of %d bytes is too large", size)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(p.r, payload); err != nil {
		return wireEvent{}, err
	}
	if hdr[0]&endStreamFlag != 0 {
		return wireEvent{}, endStreamErr(payload)
	}
	var ev wireEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		return wireEvent{}, fmt.Errorf("decode stream message: %w", err)
	}
	return ev, nil
}

// endStreamErr reads the last message of a stream: io.EOF when it carries no
// error.
func endStreamErr(payload []byte) error {
	var end struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(payload, &end) != nil || end.Error == nil {
		return io.EOF
	}
	msg := end.Error.Message
	if end.Error.Code != "" {
		msg = end.Error.Code + ": " + msg
	}
	return &APIError{Message: strings.TrimSpace(msg)}
}

func (p *httpProcess) Close() {
	p.closeOnce.Do(func() {
		p.cancel()
		_ = p.body.Close()
	})
}

type selector struct {
	PID uint32 `json:"pid"`
}

type inputBody struct {
	Stdin []byte `json:"stdin,omitempty"`
	PTY   []byte `json:"pty,omitempty"`
}

func (p *httpProcess) SendInput(ctx context.Context, data []byte) error {
	in := inputBody{Stdin: data}
	if p.pty {
		in = inputBody{PTY: data}
	}
	return p.unary(ctx, "SendInput", map[string]any{"process": selector{PID: p.pid}, "input": in})
}

func (p *httpProcess) CloseStdin(ctx context.Context) error {
	if p.pty {
		return nil
	}
	return p.unary(ctx, "CloseStdin", map[string]any{"process": selector{PID: p.pid}})
}

func (p *httpProcess) Resize(ctx context.Context, rows, cols uint16) error {
	return p.unary(ctx, "Update", map[string]any{
		"process": selector{PID: p.pid},
		"pty":     ptyBody{Size: ptySize{Cols: cols, Rows: rows}},
	})
}

func (p *httpProcess) unary(ctx context.Context, method string, body any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.e.baseURL(p.conn)+processSvc+method, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	p.e.setHeaders(req.Header, p.conn, p.user)
	resp, err := p.e.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return apiErrorOf(resp)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBody))
	return nil
}

func (e *httpEnvd) Upload(ctx context.Context, conn Connection, user, path string, body io.Reader) error {
	q := url.Values{"path": {path}, "username": {user}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL(conn)+"/files?"+q.Encode(), body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	e.setHeaders(req.Header, conn, user)
	resp, err := e.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return apiErrorOf(resp)
	}
	return nil
}
