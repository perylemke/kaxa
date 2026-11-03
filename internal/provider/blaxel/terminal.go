package blaxel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/perylemke/kaxa/internal/sandbox"
)

const (
	defaultTermRows = 24
	defaultTermCols = 80
	termReadLimit   = 1 << 20
	termChunk       = 32 << 10
	termHandshake   = 15 * time.Second

	termPath = "terminal/ws"

	termInput  = "input"
	termOutput = "output"
	termError  = "error"
	termResize = "resize"
)

// termSpec is one attachment to the sandbox terminal.
type termSpec struct {
	dir            string
	stdin          io.Reader
	stdout, stderr io.Writer
	rows, cols     uint16
	resize         <-chan sandbox.TermSize
}

// wsDialFunc dials the terminal WebSocket. Tests replace it.
type wsDialFunc func(ctx context.Context, endpoint string, header http.Header) (*websocket.Conn, *http.Response, error)

// termMessage is one JSON frame on the terminal socket.
type termMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
}

// Terminal attaches to the sandbox terminal. Blaxel starts the shell.
func (s *sdkClient) Terminal(ctx context.Context, name string, spec termSpec) error {
	inst, err := s.instance(ctx, name)
	if err != nil {
		return err
	}
	return s.attachTerminal(ctx, inst.Metadata.URL, spec)
}

func (s *sdkClient) attachTerminal(ctx context.Context, base string, spec termSpec) error {
	session, err := procName()
	if err != nil {
		return err
	}
	rows, cols := termSize(spec.rows, spec.cols)
	endpoint, err := terminalURL(base, s.apiKey, session, rows, cols, spec.dir)
	if err != nil {
		return err
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer "+s.apiKey)
	header.Set("X-Blaxel-Authorization", "Bearer "+s.apiKey)
	if s.workspace != "" {
		header.Set("X-Blaxel-Workspace", s.workspace)
	}
	conn, resp, err := s.dialTerm(ctx, endpoint, header)
	if resp != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		return s.redact(err)
	}
	return bridgeTerminal(ctx, conn, spec)
}

func (s *sdkClient) dialTerm(ctx context.Context, endpoint string, header http.Header) (*websocket.Conn, *http.Response, error) {
	if s.termDial != nil {
		return s.termDial(ctx, endpoint, header)
	}
	dialer := websocket.Dialer{HandshakeTimeout: termHandshake, Proxy: http.ProxyFromEnvironment}
	// The host was checked by terminalURL. The URL is not logged.
	conn, resp, err := dialer.DialContext(ctx, endpoint, header) //nolint:bodyclose // closed by the caller when the handshake fails
	return conn, resp, err
}

func terminalURL(base, token, session string, rows, cols uint16, dir string) (string, error) {
	if !identPattern.MatchString(session) {
		return "", errors.New("blaxel: invalid terminal session")
	}
	endpoint, err := sandboxEndpoint(base, termPath)
	if err != nil {
		return "", err
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	u.Scheme = "wss"
	q := u.Query()
	q.Set("token", token)
	q.Set("cols", strconv.Itoa(int(cols)))
	q.Set("rows", strconv.Itoa(int(rows)))
	q.Set("sessionId", session)
	q.Set("workingDir", dir)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func termSize(rows, cols uint16) (uint16, uint16) {
	if rows == 0 {
		rows = defaultTermRows
	}
	if cols == 0 {
		cols = defaultTermCols
	}
	return rows, cols
}

// termConn serializes writes. gorilla/websocket allows one writer at a time.
type termConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *termConn) write(msg termMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(msg)
}

func (c *termConn) close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	msg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
	return c.conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(time.Second))
}

func bridgeTerminal(ctx context.Context, conn *websocket.Conn, spec termSpec) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer func() { _ = conn.Close() }()
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
	tc := &termConn{conn: conn}
	go func() { _ = pumpTermInput(tc, spec.stdin) }()
	go func() { _ = watchTermResize(ctx, tc, spec.resize) }()
	return readTerminal(ctx, conn, spec.stdout, spec.stderr)
}

func pumpTermInput(tc *termConn, in io.Reader) error {
	if in == nil {
		return nil
	}
	buf := make([]byte, termChunk)
	for {
		n, err := in.Read(buf)
		if n > 0 {
			if werr := tc.write(termMessage{Type: termInput, Data: string(buf[:n])}); werr != nil {
				return werr
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			return tc.close()
		}
		return err
	}
}

func watchTermResize(ctx context.Context, tc *termConn, resize <-chan sandbox.TermSize) error {
	if resize == nil {
		<-ctx.Done()
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case size, ok := <-resize:
			if !ok {
				return nil
			}
			if size.Rows == 0 || size.Cols == 0 {
				continue
			}
			if err := tc.write(termMessage{Type: termResize, Cols: size.Cols, Rows: size.Rows}); err != nil {
				return err
			}
		}
	}
}

func readTerminal(ctx context.Context, conn *websocket.Conn, stdout, stderr io.Writer) error {
	conn.SetReadLimit(termReadLimit)
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if terminalClosed(err) {
				return nil
			}
			return err
		}
		var msg termMessage
		if json.Unmarshal(payload, &msg) != nil {
			continue
		}
		switch msg.Type {
		case termOutput:
			writeOut(stdout, msg.Data)
		case termError:
			writeOut(stderr, msg.Data)
			return terminalFailure(msg.Data)
		}
	}
}

func terminalFailure(data string) error {
	if data == "" {
		return errors.New("blaxel: terminal failed")
	}
	return fmt.Errorf("blaxel: terminal failed: %s", trimOutput(data))
}

func terminalClosed(err error) bool {
	if err == nil {
		return false
	}
	var closed *websocket.CloseError
	if errors.As(err, &closed) || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "broken pipe")
}
