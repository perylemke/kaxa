package vercel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

// The interactive terminal is hosted by the Vercel controller, not inside the
// sandbox. kaxa asks the API for a WebSocket URL and a token, then speaks the
// protocol of the Vercel Sandbox CLI (sandbox connect): a JSON "start" frame,
// binary frames for terminal input and output, JSON "resize" frames from the
// client, and a JSON "exit" frame from the server.
const (
	consoleHealthTries   = 5
	consoleHealthBackoff = 100 * time.Millisecond
	consoleDialTimeout   = 30 * time.Second
	consoleExtendEvery   = renewBelow / 2
	consoleReadChunk     = 4096
	defaultRows          = 24
	defaultCols          = 80
	termType             = "xterm-256color"
	// consolePrompt is the bash prompt: user, host, directory.
	consolePrompt = `\u@\h:\w\$ `
	// shellScript puts the kaxa toolchains ahead of the PATH and starts bash,
	// or sh when bash is missing. It is fixed text without user input.
	shellScript = `export PATH="$1:$PATH"; ` +
		`if command -v bash >/dev/null 2>&1; then exec bash --norc -i; fi; exec sh -i`
)

// consoleSpec describes one interactive session.
type consoleSpec struct {
	argv   []string
	dir    string
	env    map[string]string
	size   sandbox.TermSize
	stdin  io.Reader
	stdout io.Writer
	resize <-chan sandbox.TermSize
	// extendEvery is how often the session is extended. Zero means the default.
	extendEvery time.Duration
}

type startFrame struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Env     []string `json:"env"`
	Cwd     string   `json:"cwd"`
	Cols    uint16   `json:"cols"`
	Rows    uint16   `json:"rows"`
}

type resizeFrame struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

type controlFrame struct {
	Type    string `json:"type"`
	Code    *int   `json:"code"`
	Message string `json:"message"`
}

// errNoInteractive is returned when the project has no interactive endpoint.
var errNoInteractive = usererr.New(
	"this Vercel project does not offer an interactive terminal",
	"use kaxa exec to run commands",
)

// consoleSpecFor builds the start command of a terminal. A plain "bash" means
// the login shell; anything else runs as given with the toolchain PATH.
func consoleSpecFor(opts sandbox.ExecOptions, dir string) consoleSpec {
	spec := consoleSpec{
		dir: dir, stdin: opts.Stdin, stdout: opts.Stdout, resize: opts.Resize, size: opts.Size,
		env: map[string]string{"TERM": termType, "PS1": consolePrompt},
	}
	if len(opts.Command) == 1 && opts.Command[0] == shellBash {
		spec.argv = []string{"sh", "-c", shellScript, wrapperArg0, toolPath()}
		return spec
	}
	spec.argv = append([]string{shellBash, "-c", pathScript, wrapperArg0, toolPath()}, opts.Command...)
	return spec
}

// console runs an interactive terminal until the shell exits, the connection
// closes, or ctx ends. A non-zero exit is returned as *sandbox.ExitError.
func (s *session) console(ctx context.Context, spec consoleSpec) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	in, err := s.api.OpenInteractive(ctx, s.rec.SessionID)
	if err != nil {
		return s.consoleErr(err, "")
	}
	secrets := append([]string{in.Token}, s.secrets...)
	target, err := parseConsoleURL(in)
	if err != nil {
		return userErr(err, s.rec.Name, secrets...)
	}
	if err := waitHealthy(ctx, target.health); err != nil {
		return consoleFailure(ctx, err, s.rec.Name, secrets)
	}
	dialer := websocket.Dialer{HandshakeTimeout: consoleDialTimeout}
	conn, _, err := dialer.DialContext(ctx, target.ws, nil)
	if err != nil {
		return consoleFailure(ctx, err, s.rec.Name, secrets)
	}
	defer func() { _ = conn.Close() }()

	w := &frameWriter{conn: conn}
	if err := w.json(startOf(spec)); err != nil {
		return consoleFailure(ctx, err, s.rec.Name, secrets)
	}
	go func() { // unblock the read loop when ctx ends
		<-ctx.Done()
		_ = conn.Close()
	}()
	go pumpTerminalInput(ctx, w, spec.stdin)
	go pumpResize(ctx, w, spec.resize)
	go s.keepAlive(ctx, spec.extendEvery)

	return readTerminal(ctx, conn, spec.stdout, s.rec.Name, secrets)
}

func startOf(spec consoleSpec) startFrame {
	rows, cols := spec.size.Rows, spec.size.Cols
	if rows == 0 || cols == 0 {
		rows, cols = defaultRows, defaultCols
	}
	env := make([]string, 0, len(spec.env))
	for k, v := range spec.env {
		env = append(env, k+"="+v)
	}
	return startFrame{
		Type: "start", Command: spec.argv[0], Args: spec.argv[1:],
		Env: env, Cwd: spec.dir, Cols: cols, Rows: rows,
	}
}

// consoleErr maps a failed interactive request to a user error.
func (s *session) consoleErr(err error, token string) error {
	if api, ok := errors.AsType[*APIError](err); ok {
		switch api.Status {
		case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
			return errNoInteractive
		}
	}
	return userErr(err, s.rec.Name, append([]string{token}, s.secrets...)...)
}

// consoleFailure reports a cancelled context as such and everything else as a
// connection problem with the secrets removed.
func consoleFailure(ctx context.Context, err error, name string, secrets []string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return userErr(err, name, secrets...)
}

type consoleTarget struct {
	ws     string
	health string
}

// parseConsoleURL checks the endpoint from the API before kaxa connects to it:
// a secure WebSocket (a plain one only to loopback, for tests), a host, and no
// credentials in the URL. The token goes in the query of the dial URL only.
func parseConsoleURL(in Interactive) (consoleTarget, error) {
	u, err := url.Parse(in.URL)
	if err != nil || u.Host == "" || u.User != nil || in.Token == "" {
		return consoleTarget{}, errors.New("vercel: invalid interactive endpoint")
	}
	healthScheme := "https"
	switch u.Scheme {
	case "wss":
	case "ws":
		if !isLoopback(u.Hostname()) {
			return consoleTarget{}, errors.New("vercel: interactive endpoint is not secure")
		}
		healthScheme = "http"
	default:
		return consoleTarget{}, errors.New("vercel: invalid interactive endpoint")
	}
	q := u.Query()
	q.Set("token", in.Token)
	u.RawQuery = q.Encode()
	health := url.URL{Scheme: healthScheme, Host: u.Host, Path: "/health"}
	return consoleTarget{ws: u.String(), health: health.String()}, nil
}

// waitHealthy waits until the terminal server answers. A server in another
// region can need a moment after the API hands out its address.
func waitHealthy(ctx context.Context, healthURL string) error {
	var last error
	for attempt := 1; attempt <= consoleHealthTries; attempt++ {
		if last = checkHealth(ctx, healthURL); last == nil {
			return nil
		}
		if err := sleep(ctx, consoleHealthBackoff*time.Duration(attempt)); err != nil {
			return err
		}
	}
	return last
}

func checkHealth(ctx context.Context, healthURL string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("terminal server is not ready: status %d", resp.StatusCode)
	}
	return nil
}

// frameWriter serialises writes: gorilla/websocket allows one writer.
type frameWriter struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (w *frameWriter) json(v any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteJSON(v)
}

func (w *frameWriter) binary(b []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteMessage(websocket.BinaryMessage, b)
}

func pumpTerminalInput(ctx context.Context, w *frameWriter, r io.Reader) {
	if r == nil {
		return
	}
	buf := make([]byte, consoleReadChunk)
	for ctx.Err() == nil {
		n, err := r.Read(buf)
		if n > 0 {
			if werr := w.binary(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func pumpResize(ctx context.Context, w *frameWriter, ch <-chan sandbox.TermSize) {
	if ch == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case size, ok := <-ch:
			if !ok {
				return
			}
			if err := w.json(resizeFrame{Type: "resize", Cols: size.Cols, Rows: size.Rows}); err != nil {
				return
			}
		}
	}
}

// keepAlive extends the session while the terminal is open, so a long
// interactive shell does not end when the 30 minutes run out.
func (s *session) keepAlive(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = consoleExtendEvery
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// A failed extension is not fatal: the shell keeps running until
			// Vercel stops the session.
			_ = s.api.ExtendTimeout(ctx, s.rec.SessionID, sessionTimeout)
		}
	}
}

// readTerminal copies output to stdout until the server sends the exit frame
// or closes the connection.
func readTerminal(ctx context.Context, conn *websocket.Conn, stdout io.Writer, name string, secrets []string) error {
	for {
		kind, data, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return nil
			}
			return userErr(err, name, secrets...)
		}
		if kind == websocket.BinaryMessage {
			writeOut(stdout, data)
			continue
		}
		var frame controlFrame
		if json.Unmarshal(data, &frame) != nil {
			writeOut(stdout, data) // a text frame that is not JSON is output
			continue
		}
		switch frame.Type {
		case "exit":
			if frame.Code != nil && *frame.Code != 0 {
				return &sandbox.ExitError{Code: *frame.Code}
			}
			return nil
		case "error":
			msg := strings.TrimSpace(frame.Message)
			if msg == "" {
				msg = "the terminal reported an error"
			}
			return userErr(errors.New(msg), name, secrets...)
		}
	}
}

func writeOut(w io.Writer, b []byte) {
	if w != nil {
		// A closed terminal is not an error worth stopping the session for.
		_, _ = w.Write(b)
	}
}
