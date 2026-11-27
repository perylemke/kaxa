package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
)

const terminalWait = 5 * time.Second

// shellFake is a provider whose Exec echoes the terminal input back, like a shell.
type shellFake struct {
	*fakeProvider
	mu      sync.Mutex
	opts    sandbox.ExecOptions
	execErr error
	resized chan sandbox.TermSize
}

func newShellFake(name string) *shellFake {
	return &shellFake{fakeProvider: &fakeProvider{name: name}, resized: make(chan sandbox.TermSize, 16)}
}

func (f *shellFake) Exec(ctx context.Context, _ sandbox.ID, opts sandbox.ExecOptions) error {
	f.mu.Lock()
	f.opts = opts
	f.mu.Unlock()
	if f.execErr != nil {
		return f.execErr
	}
	go func() {
		for {
			select {
			case size := <-opts.Resize:
				f.resized <- size
			case <-ctx.Done():
				return
			}
		}
	}()
	_, err := io.Copy(opts.Stdout, opts.Stdin)
	return err
}

func (f *shellFake) lastOpts() sandbox.ExecOptions {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.opts
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// socketServer serves the handler on a real loopback port, because a WebSocket
// needs a connection. The server's host is set before it starts.
func socketServer(t *testing.T, p sandbox.Provider, opts Options) (*Server, *syncBuffer) {
	t.Helper()
	stderr := &syncBuffer{}
	opts.Stderr = stderr
	s, h := newServer(t, p, opts)
	ts := httptest.NewUnstartedServer(h)
	s.host = ts.Listener.Addr().String()
	ts.Start()
	t.Cleanup(ts.Close)
	return s, stderr
}

func ticketFor(t *testing.T, s *Server, body any) terminalOut {
	t.Helper()
	rec := do(t, s.Handler(), s, http.MethodPost, "/api/sandboxes/lab/box/terminal", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("terminal = %d %s", rec.Code, rec.Body)
	}
	var out terminalOut
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func dialTerminal(s *Server, ticket, query, origin string) (*websocket.Conn, *http.Response, error) {
	header := http.Header{}
	if origin != "" {
		header.Set("Origin", origin)
	}
	return websocket.DefaultDialer.Dial("ws://"+s.host+"/ws/terminal?ticket="+ticket+query, header)
}

func mustDial(t *testing.T, s *Server, ticket, query string) *websocket.Conn {
	t.Helper()
	conn, resp, err := dialTerminal(s, ticket, query, "http://"+s.host)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func readUntil(t *testing.T, conn *websocket.Conn, want string) string {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(terminalWait))
	var got strings.Builder
	for !strings.Contains(got.String(), want) {
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v (got %q, want %q)", err, got.String(), want)
		}
		got.Write(data)
	}
	return got.String()
}

func waitForLog(t *testing.T, stderr *syncBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(terminalWait)
	for time.Now().Before(deadline) {
		if strings.Contains(stderr.String(), want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("log is missing %q\n%s", want, stderr.String())
}

func TestTerminalTicketsAreSingleUseAndExpire(t *testing.T) {
	t.Parallel()

	var tickets terminals
	now := fixedNow
	first, err := tickets.issue(testProvider, testSandbox, now)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := tickets.redeem(first, now.Add(time.Second))
	if !ok || got.provider != testProvider || got.name != testSandbox {
		t.Fatalf("redeem = %+v %v", got, ok)
	}
	if _, ok := tickets.redeem(first, now.Add(time.Second)); ok {
		t.Fatal("a ticket was accepted twice")
	}
	late, _ := tickets.issue(testProvider, testSandbox, now)
	if _, ok := tickets.redeem(late, now.Add(ticketTTL+time.Second)); ok {
		t.Fatal("an expired ticket was accepted")
	}
	if _, ok := tickets.redeem("not-hex", now); ok {
		t.Fatal("a bad ticket was accepted")
	}
}

func TestTerminalTicketsAreBounded(t *testing.T) {
	t.Parallel()

	var tickets terminals
	first, _ := tickets.issue(testProvider, testSandbox, fixedNow)
	for range maxTickets {
		if _, err := tickets.issue(testProvider, testSandbox, fixedNow); err != nil {
			t.Fatal(err)
		}
	}
	if len(tickets.tickets) != maxTickets {
		t.Fatalf("pending = %d", len(tickets.tickets))
	}
	if _, ok := tickets.redeem(first, fixedNow); ok {
		t.Fatal("the oldest ticket survived")
	}
	// An expired ticket is dropped when the next one is issued.
	_, _ = tickets.issue(testProvider, testSandbox, fixedNow.Add(time.Hour))
	if len(tickets.tickets) != 1 {
		t.Fatalf("pending after expiry = %d", len(tickets.tickets))
	}
}

func TestOpenTerminalReturnsATicketAndTheCommand(t *testing.T) {
	t.Parallel()

	s, _ := newServer(t, newShellFake(testProvider), Options{})
	out := ticketFor(t, s, map[string]string{})
	if out.Ticket == "" || out.Command != "kaxa --provider lab ssh box" {
		t.Fatalf("out = %+v", out)
	}
}

func TestOpenTerminalRefuses(t *testing.T) {
	t.Parallel()

	archived := newShellFake(testProvider)
	archived.detail = sandbox.Detail{ID: testSandbox, Status: statusArch}
	broken := newShellFake(testProvider)
	broken.detail = sandbox.Detail{ID: testSandbox, Status: statusError}
	missing := newShellFake(testProvider)
	missing.inspectErr = stringError("sandbox box not found")
	cases := []struct {
		name string
		p    sandbox.Provider
		path string
		body any
		code int
	}{
		{name: "bad body", p: newShellFake(testProvider), path: testSandbox, body: map[string]int{"other": 1}, code: http.StatusBadRequest},
		{name: "archived", p: archived, path: testSandbox, body: map[string]string{}, code: http.StatusBadRequest},
		{name: "error state", p: broken, path: testSandbox, body: map[string]string{}, code: http.StatusBadRequest},
		{name: "not found", p: missing, path: testSandbox, body: map[string]string{}, code: http.StatusNotFound},
		{name: "bad sandbox name", p: newShellFake(testProvider), path: "BOX", body: map[string]string{}, code: http.StatusBadRequest},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, h := newServer(t, tt.p, Options{})
			rec := do(t, h, s, http.MethodPost, "/api/sandboxes/lab/"+tt.path+"/terminal", tt.body)
			if rec.Code != tt.code {
				t.Fatalf("status = %d %s", rec.Code, rec.Body)
			}
			if len(s.term.tickets) != 0 {
				t.Fatal("a refused request issued a ticket")
			}
		})
	}
}

func TestOpenTerminalReportsATokenFailure(t *testing.T) {
	s, h := newServer(t, newShellFake(testProvider), Options{})
	prev := randRead
	randRead = func([]byte) (int, error) { return 0, errors.New("no entropy") }
	t.Cleanup(func() { randRead = prev })
	rec := do(t, h, s, http.MethodPost, "/api/sandboxes/lab/box/terminal", map[string]string{})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
}

func TestTerminalBridgesKeystrokesAndOutput(t *testing.T) {
	t.Parallel()

	fp := newShellFake(testProvider)
	s, stderr := socketServer(t, fp, Options{})
	out := ticketFor(t, s, map[string]string{})
	conn := mustDial(t, s, out.Ticket, "&rows=30&cols=100")

	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("ls\n")); err != nil {
		t.Fatal(err)
	}
	readUntil(t, conn, "ls\n")
	opts := fp.lastOpts()
	if !opts.TTY || opts.Size != (sandbox.TermSize{Rows: 30, Cols: 100}) || len(opts.Command) != 1 || opts.Command[0] != "bash" {
		t.Fatalf("opts = %+v", opts)
	}

	checkResizeFrames(t, conn, fp)
	_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	waitForLog(t, stderr, "closed after")
	checkTerminalLog(t, stderr, out.Ticket)
	if len(fp.resized) != 0 {
		t.Fatalf("invalid resize frames reached the provider: %d", len(fp.resized))
	}
}

func checkResizeFrames(t *testing.T, conn *websocket.Conn, fp *shellFake) {
	t.Helper()
	for _, frame := range []string{`{"resize":{"rows":40,"cols":120}}`, "not json", `{"other":1}`, `{"resize":{"rows":0,"cols":5}}`, `{"resize":{"rows":5,"cols":9999}}`} {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case size := <-fp.resized:
		if size != (sandbox.TermSize{Rows: 40, Cols: 120}) {
			t.Fatalf("resize = %+v", size)
		}
	case <-time.After(terminalWait):
		t.Fatal("the resize never reached the provider")
	}
}

func checkTerminalLog(t *testing.T, stderr *syncBuffer, ticket string) {
	t.Helper()
	log := stderr.String()
	if !strings.Contains(log, "terminal  lab/box") || !strings.Contains(log, "opened") {
		t.Fatalf("log = %s", log)
	}
	if strings.Contains(log, ticket) || strings.Contains(log, "ls") {
		t.Fatalf("log repeats the ticket or the session\n%s", log)
	}
}

func TestTerminalFallsBackToDefaultSize(t *testing.T) {
	t.Parallel()

	fp := newShellFake(testProvider)
	s, stderr := socketServer(t, fp, Options{})
	conn := mustDial(t, s, ticketFor(t, s, map[string]string{}).Ticket, "&rows=0&cols=x")
	_ = conn.WriteMessage(websocket.BinaryMessage, []byte("x"))
	readUntil(t, conn, "x")
	if got := fp.lastOpts(); got.Size != (sandbox.TermSize{Rows: 24, Cols: 80}) {
		t.Fatalf("opts = %+v", got)
	}
	_ = conn.Close()
	waitForLog(t, stderr, "closed after")
}

func TestTerminalShowsAnExecErrorAndCloses(t *testing.T) {
	t.Parallel()

	fp := newShellFake(testProvider)
	fp.execErr = stringError("shell failed")
	s, stderr := socketServer(t, fp, Options{})
	conn := mustDial(t, s, ticketFor(t, s, map[string]string{}).Ticket, "")
	readUntil(t, conn, "kaxa: shell failed")
	_ = conn.SetReadDeadline(time.Now().Add(terminalWait))
	if _, _, err := conn.ReadMessage(); !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
		t.Fatalf("err = %v", err)
	}
	waitForLog(t, stderr, "error  terminal  lab/box")
}

func TestTerminalRejectsBadHandshakes(t *testing.T) {
	t.Parallel()

	s, stderr := socketServer(t, newShellFake(testProvider), Options{})
	good := ticketFor(t, s, map[string]string{}).Ticket
	expired, _ := s.term.issue(testProvider, testSandbox, time.Now().Add(-time.Hour))
	cases := []struct {
		name   string
		ticket string
		origin string
	}{
		{name: "no origin", ticket: good},
		{name: "foreign origin", ticket: good, origin: "http://evil.example"},
		{name: "unknown ticket", ticket: "00", origin: "http://" + s.host},
		{name: "expired ticket", ticket: expired, origin: "http://" + s.host},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, resp, err := dialTerminal(s, tt.ticket, "", tt.origin)
			if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
				t.Fatalf("dial = %v %v", resp, err)
			}
			_ = resp.Body.Close()
		})
	}
	log := stderr.String()
	for _, want := range []string{"rejected origin", "invalid terminal ticket"} {
		if !strings.Contains(log, want) {
			t.Fatalf("log is missing %q\n%s", want, log)
		}
	}
	if strings.Contains(log, good) || strings.Contains(log, "evil.example") {
		t.Fatalf("log repeats caller input\n%s", log)
	}
	// The good ticket was never redeemed, so it still works.
	_ = mustDial(t, s, good, "")
}

func TestTerminalRefusesWhatItCannotOpen(t *testing.T) {
	t.Parallel()

	s, _ := socketServer(t, newShellFake(testProvider), Options{Provider: testProvider})
	other, _ := s.term.issue(testOther, testSandbox, time.Now())
	notUpgrade, _ := s.term.issue(testProvider, testSandbox, time.Now())
	for name, ticket := range map[string]string{"other provider": other} {
		_, resp, err := dialTerminal(s, ticket, "", "http://"+s.host)
		if err == nil || resp == nil || resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: dial = %v %v", name, resp, err)
		}
		_ = resp.Body.Close()
	}
	req, _ := http.NewRequest(http.MethodGet, "http://"+s.host+"/ws/terminal?ticket="+notUpgrade, nil)
	req.Header.Set("Origin", "http://"+s.host)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("plain GET = %d", resp.StatusCode)
	}
}

func TestKeepAlivePingsUntilTheSocketCloses(t *testing.T) {
	prev := wsPingEvery
	wsPingEvery = 10 * time.Millisecond
	t.Cleanup(func() { wsPingEvery = prev })

	fp := newShellFake(testProvider)
	s, stderr := socketServer(t, fp, Options{})
	conn := mustDial(t, s, ticketFor(t, s, map[string]string{}).Ticket, "")
	pinged := make(chan struct{}, 1)
	conn.SetPingHandler(func(string) error {
		select {
		case pinged <- struct{}{}:
		default:
		}
		return conn.WriteControl(websocket.PongMessage, nil, time.Now().Add(time.Second))
	})
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	select {
	case <-pinged:
	case <-time.After(terminalWait):
		t.Fatal("no ping")
	}
	// Let the server read a pong, then end the shell.
	time.Sleep(50 * time.Millisecond)
	_ = conn.Close()
	waitForLog(t, stderr, "closed after")
}

func TestKeepAliveStopsWhenTheSocketIsGone(t *testing.T) {
	prev := wsPingEvery
	wsPingEvery = 5 * time.Millisecond
	t.Cleanup(func() { wsPingEvery = prev })

	// A closed connection makes the next ping fail, which ends keepAlive.
	local := dialLocal(t)
	_ = local.Close()
	done := make(chan struct{})
	go func() {
		keepAlive(t.Context(), local)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(terminalWait):
		t.Fatal("keepAlive did not stop")
	}
}

func TestApplyResizeDropsWhenTheQueueIsFull(t *testing.T) {
	t.Parallel()

	full := make(chan sandbox.TermSize)
	applyResize([]byte(`{"resize":{"rows":10,"cols":10}}`), full)
	if len(full) != 0 {
		t.Fatal("a full queue must not block or grow")
	}
}

func TestShellCommandUsesSSH(t *testing.T) {
	t.Parallel()

	if got := shellCommand("e2b", testSandbox); got != "kaxa --provider e2b ssh box" {
		t.Fatalf("command = %q", got)
	}
}

func TestInspectCarriesTheShellCommand(t *testing.T) {
	t.Parallel()

	reg := registry.New()
	mustRegister(t, reg, newShellFake(testProvider))
	s, h := newServer(t, nil, Options{Registry: reg})
	rec := do(t, h, s, http.MethodGet, "/api/sandboxes/lab/box", nil)
	var got detailJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.ShellCommand != "kaxa --provider lab ssh box" {
		t.Fatalf("detail = %+v %v", got, err)
	}
}

// dialLocal returns a client WebSocket connected to a throwaway echo server.
func dialLocal(t *testing.T) *websocket.Conn {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := terminalUpgrader.Upgrade(w, r, nil)
		if err == nil {
			_ = conn.Close()
		}
	}))
	t.Cleanup(ts.Close)
	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http"), nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("stdin closed") }

func TestPumpSocketStopsWhenStdinFails(t *testing.T) {
	t.Parallel()

	done := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := terminalUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		pumpSocket(conn, failingWriter{}, make(chan sandbox.TermSize, 1))
		close(done)
	}))
	t.Cleanup(ts.Close)
	conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http"), nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte("x")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(terminalWait):
		t.Fatal("pumpSocket kept reading after stdin failed")
	}
}

func TestSocketWriterReportsAClosedSocket(t *testing.T) {
	t.Parallel()

	conn := dialLocal(t)
	_ = conn.Close()
	out := &socketWriter{conn: conn}
	if _, err := out.Write([]byte("x")); err == nil {
		t.Fatal("write to a closed socket succeeded")
	}
}

func TestVendoredTerminalAssetsAreServed(t *testing.T) {
	t.Parallel()

	s, h := newServer(t, newShellFake(testProvider), Options{})
	cases := map[string]string{
		"/vendor/xterm.js":     contentJS,
		"/vendor/xterm.css":    contentCSS,
		"/vendor/addon-fit.js": contentJS,
	}
	for path, kind := range cases {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Host = s.host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != kind || rec.Body.Len() == 0 {
			t.Fatalf("%s = %d %q", path, rec.Code, rec.Header().Get("Content-Type"))
		}
	}
}

func TestTerminalTreatsAShellExitCodeAsANormalEnd(t *testing.T) {
	t.Parallel()

	fp := newShellFake(testProvider)
	fp.execErr = &sandbox.ExitError{Code: 3}
	s, stderr := socketServer(t, fp, Options{})
	conn := mustDial(t, s, ticketFor(t, s, map[string]string{}).Ticket, "")
	_ = conn.SetReadDeadline(time.Now().Add(terminalWait))
	if _, _, err := conn.ReadMessage(); !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
		t.Fatalf("err = %v", err)
	}
	waitForLog(t, stderr, "shell exited with code 3")
	if strings.Contains(stderr.String(), "error  terminal") {
		t.Fatalf("an exit code was logged as an error\n%s", stderr)
	}
}

func TestTerminalOpensIndependentShellsOnTheSameSandbox(t *testing.T) {
	t.Parallel()

	s, stderr := socketServer(t, newShellFake(testProvider), Options{})
	first := mustDial(t, s, ticketFor(t, s, map[string]string{}).Ticket, "")
	second := mustDial(t, s, ticketFor(t, s, map[string]string{}).Ticket, "")
	for _, conn := range []*websocket.Conn{first, second} {
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte("hi"))
		readUntil(t, conn, "hi")
	}
	_ = first.Close()
	_ = second.Close()
	waitForLog(t, stderr, "closed after")
}
