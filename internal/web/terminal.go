package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	ticketTTL     = 30 * time.Second
	maxTickets    = 16
	wsReadLimit   = 64 << 10
	wsDeadAfter   = 90 * time.Second
	wsCloseWait   = time.Second
	wsBuffer      = 4096
	termRows      = 24
	termCols      = 80
	termMaxSize   = 500
	resizeQueue   = 8
	statusError   = "error"
	statusArch    = "archived"
	statusStopped = "stopped"
)

// terminalTicket lets one WebSocket open a terminal on one sandbox. It is random,
// single use, and short lived, because a browser cannot send the session headers
// on a WebSocket.
// wsPingEvery is how often the page is pinged; tests shorten it.
var wsPingEvery atomic.Int64

func init() { wsPingEvery.Store(int64(30 * time.Second)) }

type terminalTicket struct {
	secret   []byte
	provider string
	name     string
	expires  time.Time
}

// terminals holds the pending tickets.
type terminals struct {
	mu      sync.Mutex
	tickets []terminalTicket
}

// issue stores a ticket and returns its hex form.
func (t *terminals) issue(provider, name string, now time.Time) (string, error) {
	raw, encoded, err := newToken()
	if err != nil {
		return "", err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	live := make([]terminalTicket, 0, len(t.tickets)+1)
	for _, ticket := range t.tickets {
		if now.Before(ticket.expires) {
			live = append(live, ticket)
		}
	}
	if len(live) >= maxTickets {
		live = live[1:]
	}
	live = append(live, terminalTicket{
		secret: raw, provider: provider, name: name, expires: now.Add(ticketTTL),
	})
	t.tickets = live
	return encoded, nil
}

// redeem finds the ticket for encoded, removes it, and reports whether it was still valid.
// Every pending ticket is compared in constant time.
func (t *terminals) redeem(encoded string, now time.Time) (terminalTicket, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	found := -1
	for i, ticket := range t.tickets {
		if matchToken(encoded, ticket.secret) {
			found = i
		}
	}
	if found < 0 {
		return terminalTicket{}, false
	}
	ticket := t.tickets[found]
	t.tickets = append(t.tickets[:found], t.tickets[found+1:]...)
	return ticket, now.Before(ticket.expires)
}

type terminalOut struct {
	Ticket  string `json:"ticket"`
	Command string `json:"command"`
}

// shellCommand is the kaxa command that opens the same shell in a local terminal.
func shellCommand(provider, name string) string {
	return "kaxa --provider " + provider + " ssh " + name
}

// openTerminal checks the sandbox and hands the page a ticket for the WebSocket.
func (s *Server) openTerminal(w http.ResponseWriter, r *http.Request) {
	p, name, ok := s.sandboxFromPath(w, r)
	if !ok {
		return
	}
	var body struct{}
	if err := decodeJSON(r, &body); err != nil {
		s.writeErr(w, err)
		return
	}
	detail, err := p.Inspect(r.Context(), sandbox.ID(name))
	if err != nil {
		s.writeErr(w, err)
		return
	}
	switch detail.Status {
	case statusArch:
		s.writeErr(w, usererr.New("the sandbox is archived", "run kaxa unarchive "+name+" first"))
		return
	case statusError:
		s.writeErr(w, usererr.New("the sandbox is in an error state", "check it with kaxa list, then restart or destroy it"))
		return
	}
	ticket, err := s.term.issue(p.Name(), name, s.clock())
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, terminalOut{Ticket: ticket, Command: shellCommand(p.Name(), name)})
}

var terminalUpgrader = websocket.Upgrader{
	ReadBufferSize:  wsBuffer,
	WriteBufferSize: wsBuffer,
	// terminalSocket checks the Origin against the page address before it upgrades.
	CheckOrigin: func(*http.Request) bool { return true },
}

// terminalSocket upgrades a request that carries a valid ticket and bridges the
// WebSocket to an interactive bash in the sandbox, the same as kaxa console.
func (s *Server) terminalSocket(w http.ResponseWriter, r *http.Request) {
	// Counted before the upgrade, while Shutdown still tracks the connection.
	s.shells.Add(1)
	defer s.shells.Done()
	s.open.Add(1)
	defer s.open.Add(-1)
	if r.Header.Get("Origin") != "http://"+s.host {
		s.reject(w, http.StatusForbidden, "rejected origin", "open the address printed by kaxa ui")
		return
	}
	ticket, ok := s.term.redeem(r.URL.Query().Get("ticket"), s.clock())
	if !ok {
		s.reject(w, http.StatusForbidden, "invalid terminal ticket", "reopen the terminal from the sandbox page")
		return
	}
	target := ticket.provider + "/" + ticket.name
	p, err := s.openProvider(ticket.provider)
	if err != nil {
		_, message, hint := s.describe(err)
		s.writeStatus(w, http.StatusBadRequest, message, hint)
		return
	}
	conn, err := terminalUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	size := terminalSize(r)
	started := s.clock()
	s.event(levelInfo, "terminal", target, "opened")
	s.bridge(r.Context(), conn, p, sandbox.ID(ticket.name), size, target)
	s.event(levelInfo, "terminal", target, "closed after "+elapsed(s.clock().Sub(started)))
}

// terminalSize reads the first size the page reports, within sane limits.
func terminalSize(r *http.Request) sandbox.TermSize {
	return sandbox.TermSize{
		Rows: boundedSize(r.URL.Query().Get("rows"), termRows),
		Cols: boundedSize(r.URL.Query().Get("cols"), termCols),
	}
}

func boundedSize(raw string, fallback uint16) uint16 {
	n, err := strconv.ParseUint(raw, 10, 16)
	if err != nil || n < 1 || n > termMaxSize {
		return fallback
	}
	return uint16(n)
}

// socketWriter sends the terminal output to the page. WebSocket writes are serialized.
type socketWriter struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *socketWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// resizeFrame is the text message the page sends when its terminal changes size.
type resizeFrame struct {
	Resize *struct {
		Rows int `json:"rows"`
		Cols int `json:"cols"`
	} `json:"resize"`
}

func (s *Server) bridge(ctx context.Context, conn *websocket.Conn, p sandbox.Provider, id sandbox.ID, size sandbox.TermSize, target string) {
	parent := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stdinR, stdinW := io.Pipe()
	defer func() { _ = stdinR.Close() }()
	resize := make(chan sandbox.TermSize, resizeQueue)
	out := &socketWriter{conn: conn}
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		defer cancel()
		defer func() { _ = stdinW.Close() }()
		pumpSocket(conn, stdinW, resize)
	}()
	go keepAlive(ctx, conn)
	// A provider that reads stdin without watching ctx still ends with the shell.
	go func() {
		<-ctx.Done()
		_ = stdinR.Close()
	}()

	err := p.Exec(ctx, id, sandbox.ExecOptions{
		Command: []string{"bash"}, Stdin: stdinR, Stdout: out, Stderr: out, TTY: true, Size: size, Resize: resize,
	})
	s.reportShellEnd(ctx, out, target, err)
	code := websocket.CloseNormalClosure
	if parent.Err() != nil {
		code = websocket.CloseGoingAway
	}
	closing := websocket.FormatCloseMessage(code, "")
	_ = conn.WriteControl(websocket.CloseMessage, closing, time.Now().Add(wsCloseWait))
	_ = conn.Close()
	<-readerDone
}

// reportShellEnd logs how the shell ended. A shell that exits with a code, as
// after exit, is a normal end; any other failure is shown in the terminal.
func (s *Server) reportShellEnd(ctx context.Context, out io.Writer, target string, err error) {
	var exit *sandbox.ExitError
	switch {
	case err == nil || ctx.Err() != nil:
	case errors.As(err, &exit):
		s.event(levelInfo, "terminal", target, "shell exited with code "+strconv.Itoa(exit.Code))
	default:
		_, message, _ := s.describe(err)
		s.event(levelError, "terminal", target, message)
		_, _ = out.Write([]byte("\r\nkaxa: " + oneLine(message) + "\r\n"))
	}
}

// pumpSocket copies keystrokes to stdin and applies resize frames, until the socket ends.
func pumpSocket(conn *websocket.Conn, stdin io.Writer, resize chan<- sandbox.TermSize) {
	conn.SetReadLimit(wsReadLimit)
	extend := func() { _ = conn.SetReadDeadline(time.Now().Add(wsDeadAfter)) }
	extend()
	conn.SetPongHandler(func(string) error {
		extend()
		return nil
	})
	for {
		kind, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		extend()
		switch kind {
		case websocket.BinaryMessage:
			if _, err := stdin.Write(data); err != nil {
				return
			}
		case websocket.TextMessage:
			applyResize(data, resize)
		}
	}
}

func applyResize(data []byte, resize chan<- sandbox.TermSize) {
	var frame resizeFrame
	if json.Unmarshal(data, &frame) != nil || frame.Resize == nil {
		return
	}
	rows, cols := frame.Resize.Rows, frame.Resize.Cols
	if rows < 1 || rows > termMaxSize || cols < 1 || cols > termMaxSize {
		return
	}
	select {
	case resize <- sandbox.TermSize{Rows: uint16(rows), Cols: uint16(cols)}:
	default:
	}
}

// keepAlive pings the page so a closed tab is noticed. The browser answers on its own.
func keepAlive(ctx context.Context, conn *websocket.Conn) {
	tick := time.NewTicker(time.Duration(wsPingEvery.Load()))
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(wsCloseWait)); err != nil {
				return
			}
		}
	}
}
