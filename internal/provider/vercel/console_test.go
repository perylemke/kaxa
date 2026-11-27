package vercel

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

	"github.com/perylemke/kaxa/internal/sandbox"
)

const ptyToken = "pty-secret-token"

// fakePTY is a terminal server: /health and a WebSocket at /pty that echoes
// input and understands "exit3" and "exit0" as the shell ending.
type fakePTY struct {
	srv         *httptest.Server
	mu          sync.Mutex
	starts      []map[string]any
	resizes     []map[string]any
	input       bytes.Buffer
	healthFails int
	healthCalls int
	// script, when set, replaces the echo loop after the start frame.
	script func(c *websocket.Conn)
}

func newFakePTY(t *testing.T) *fakePTY {
	t.Helper()
	p := &fakePTY{}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		p.mu.Lock()
		p.healthCalls++
		fail := p.healthCalls <= p.healthFails
		p.mu.Unlock()
		if fail {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})
	up := websocket.Upgrader{}
	mux.HandleFunc("/pty", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != ptyToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		var start map[string]any
		if err := c.ReadJSON(&start); err != nil {
			return
		}
		p.mu.Lock()
		p.starts = append(p.starts, start)
		script := p.script
		p.mu.Unlock()
		if script != nil {
			script(c)
			return
		}
		p.echo(c)
	})
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

func (p *fakePTY) wsURL() string { return "ws" + strings.TrimPrefix(p.srv.URL, "http") + "/pty" }

func (p *fakePTY) echo(c *websocket.Conn) {
	for {
		kind, data, err := c.ReadMessage()
		if err != nil {
			return
		}
		if kind == websocket.TextMessage {
			var m map[string]any
			_ = json.Unmarshal(data, &m)
			p.mu.Lock()
			p.resizes = append(p.resizes, m)
			p.mu.Unlock()
			continue
		}
		p.mu.Lock()
		p.input.Write(data)
		p.mu.Unlock()
		_ = c.WriteMessage(websocket.BinaryMessage, data)
		switch {
		case bytes.Contains(data, []byte("exit3")):
			_ = c.WriteJSON(map[string]any{keyType: "exit", keyCode: 3})
			return
		case bytes.Contains(data, []byte("exit0")):
			_ = c.WriteJSON(map[string]any{keyType: "exit", keyCode: 0})
			return
		}
	}
}

func (f *fakeVercel) interactive(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	pty := f.pty
	f.mu.Unlock()
	if pty == nil {
		f.fail(w, http.StatusNotFound, "no interactive")
		return
	}
	f.reply(w, map[string]any{"url": pty.wsURL(), "token": ptyToken})
}

func consoleBox(t *testing.T) (*Provider, *fakeVercel, *fakePTY) {
	t.Helper()
	p, f := newTestProvider(t)
	createBox(t, p)
	pty := newFakePTY(t)
	f.mu.Lock()
	f.pty = pty
	f.mu.Unlock()
	return p, f, pty
}

//nolint:gocyclo // one scenario that checks every field of the start frame
func TestConsoleExitCodes(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		input string
		code  int
	}{
		{"zero", "ls\nexit0\n", 0},
		{"three", "ls\nexit3\n", 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, _, pty := consoleBox(t)
			var out bytes.Buffer
			resize := make(chan sandbox.TermSize, 1)
			resize <- sandbox.TermSize{Rows: 50, Cols: 120}
			err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{
				Command: []string{shellBash}, TTY: true, Stdin: strings.NewReader(tt.input),
				Stdout: &out, Size: sandbox.TermSize{Rows: 30, Cols: 100}, Resize: resize,
			})
			var exit *sandbox.ExitError
			switch {
			case tt.code == 0 && err != nil:
				t.Fatalf("Exec() = %v", err)
			case tt.code != 0 && (!errors.As(err, &exit) || exit.Code != tt.code):
				t.Fatalf("Exec() = %v, want exit %d", err, tt.code)
			}
			if !strings.Contains(out.String(), "ls") {
				t.Fatalf("output = %q, want the echoed input", out.String())
			}
			pty.mu.Lock()
			defer pty.mu.Unlock()
			if len(pty.starts) != 1 {
				t.Fatalf("starts = %d", len(pty.starts))
			}
			st := pty.starts[0]
			if st["type"] != "start" || st["command"] != "sh" || asFloat(st["rows"]) != 30 || asFloat(st["cols"]) != 100 {
				t.Fatalf("start frame = %v", st)
			}
			if st["cwd"] != vercelHome {
				t.Fatalf("cwd = %v", st["cwd"])
			}
			env := strings.Join(toStrings(asList(st["env"])), " ")
			if !strings.Contains(env, "TERM=xterm-256color") || !strings.Contains(env, "PS1=") {
				t.Fatalf("env = %q", env)
			}
		})
	}
}

func toStrings(in []any) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		out = append(out, asStr(v))
	}
	return out
}

func TestConsoleForwardsResize(t *testing.T) {
	t.Parallel()
	p, _, pty := consoleBox(t)
	resize := make(chan sandbox.TermSize, 1)
	resize <- sandbox.TermSize{Rows: 50, Cols: 120}
	// The input arrives after the resize has been sent.
	in, w := io.Pipe()
	t.Cleanup(func() { _ = w.Close() })
	go func() {
		for {
			pty.mu.Lock()
			n := len(pty.resizes)
			pty.mu.Unlock()
			if n > 0 {
				_, _ = w.Write([]byte("exit0\n"))
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{
		Command: []string{shellBash}, TTY: true, Stdin: in, Stdout: &bytes.Buffer{}, Resize: resize,
	})
	if err != nil {
		t.Fatal(err)
	}
	pty.mu.Lock()
	defer pty.mu.Unlock()
	r := pty.resizes[0]
	if r["type"] != "resize" || asFloat(r["rows"]) != 50 || asFloat(r["cols"]) != 120 {
		t.Fatalf("resize frame = %v", r)
	}
	st := pty.starts[0]
	if asFloat(st["rows"]) != defaultRows || asFloat(st["cols"]) != defaultCols {
		t.Fatalf("a zero size should start at %dx%d, got %v", defaultCols, defaultRows, st)
	}
}

func TestConsoleCustomCommandAndDir(t *testing.T) {
	t.Parallel()
	p, _, pty := consoleBox(t)
	err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{
		Command: []string{"python3", "-i"}, Dir: "/tmp", TTY: true, Stdin: strings.NewReader("exit0\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	pty.mu.Lock()
	defer pty.mu.Unlock()
	st := pty.starts[0]
	args := toStrings(asList(st["args"]))
	if st["command"] != shellBash || st["cwd"] != "/tmp" || args[len(args)-2] != "python3" || args[len(args)-1] != "-i" {
		t.Fatalf("start frame = %v", st)
	}
}

func TestConsoleWorkspaceForSyncedSandbox(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p, func(sp *sandbox.Spec) { sp.Labels = append(sp.Labels, sandbox.SyncLabel) })
	pty := newFakePTY(t)
	f.mu.Lock()
	f.pty = pty
	f.mu.Unlock()
	if err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{
		Command: []string{shellBash}, TTY: true, Stdin: strings.NewReader("exit0\n"),
	}); err != nil {
		t.Fatal(err)
	}
	pty.mu.Lock()
	defer pty.mu.Unlock()
	if pty.starts[0]["cwd"] != workspaceDir {
		t.Fatalf("cwd = %v, want %s", pty.starts[0]["cwd"], workspaceDir)
	}
}

func TestConsoleHealthIsRetried(t *testing.T) {
	t.Parallel()
	p, _, pty := consoleBox(t)
	pty.healthFails = 2
	if err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{
		Command: []string{shellBash}, TTY: true, Stdin: strings.NewReader("exit0\n"),
	}); err != nil {
		t.Fatal(err)
	}
	if pty.healthCalls != 3 {
		t.Fatalf("health calls = %d, want 3", pty.healthCalls)
	}
}

func TestConsoleHealthGivesUp(t *testing.T) {
	t.Parallel()
	p, _, pty := consoleBox(t)
	pty.healthFails = 100
	err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{Command: []string{shellBash}, TTY: true})
	if err == nil || strings.Contains(err.Error(), ptyToken) {
		t.Fatalf("Exec() = %v", err)
	}
	if pty.healthCalls != consoleHealthTries {
		t.Fatalf("health calls = %d, want %d", pty.healthCalls, consoleHealthTries)
	}
}

func TestConsoleCancelClosesSocket(t *testing.T) {
	t.Parallel()
	p, _, pty := consoleBox(t)
	pty.script = func(c *websocket.Conn) {
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := p.Exec(ctx, boxName, sandbox.ExecOptions{Command: []string{shellBash}, TTY: true})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Exec() = %v, want the context error", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("cancel did not close the terminal")
	}
}

func TestConsoleServerClosesNormally(t *testing.T) {
	t.Parallel()
	p, _, pty := consoleBox(t)
	pty.script = func(c *websocket.Conn) {
		_ = c.WriteMessage(websocket.BinaryMessage, []byte("bye"))
		_ = c.WriteMessage(websocket.TextMessage, []byte("plain text"))
		_ = c.WriteJSON(map[string]any{keyType: "ignored"})
		_ = c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}
	var out bytes.Buffer
	err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{Command: []string{shellBash}, TTY: true, Stdout: &out})
	if err != nil || out.String() != "byeplain text" {
		t.Fatalf("Exec() = %q, %v", out.String(), err)
	}
}

func TestConsoleServerAbnormalClose(t *testing.T) {
	t.Parallel()
	p, _, pty := consoleBox(t)
	pty.script = func(c *websocket.Conn) { _ = c.UnderlyingConn().Close() }
	err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{Command: []string{shellBash}, TTY: true})
	if err == nil || strings.Contains(err.Error(), ptyToken) {
		t.Fatalf("Exec() = %v, want a connection error", err)
	}
}

func TestConsoleErrorFrame(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ name, msg, want string }{
		{"message", "no such user", "no such user"},
		{"empty", "", "reported an error"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, _, pty := consoleBox(t)
			pty.script = func(c *websocket.Conn) {
				msg := tt.msg
				if msg != "" {
					msg += " " + ptyToken
				}
				_ = c.WriteJSON(map[string]any{keyType: "error", keyMessage: msg})
			}
			err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{Command: []string{shellBash}, TTY: true})
			if err == nil || !strings.Contains(err.Error(), tt.want) || strings.Contains(err.Error(), ptyToken) {
				t.Fatalf("Exec() = %v, want %q without the token", err, tt.want)
			}
		})
	}
}

func TestConsoleWithoutEndpoint(t *testing.T) {
	t.Parallel()
	p, _ := newTestProvider(t)
	createBox(t, p)
	err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{Command: []string{shellBash}, TTY: true})
	if err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("Exec() = %v, want the friendly message", err)
	}
}

func TestConsoleAPIFailure(t *testing.T) {
	t.Parallel()
	p, f, _ := consoleBox(t)
	f.fail2("POST /v2/sandboxes/sessions/{sid}/interactive", http.StatusForbidden)
	err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{Command: []string{shellBash}, TTY: true})
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("Exec() = %v", err)
	}
}

func TestParseConsoleURL(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		in     Interactive
		wantOK bool
		health string
	}{
		{"secure", Interactive{URL: "wss://pty.example.com/s/1", Token: "t"}, true, "https://pty.example.com/health"},
		{"loopback plain", Interactive{URL: "ws://127.0.0.1:9/p", Token: "t"}, true, "http://127.0.0.1:9/health"},
		{"remote plain", Interactive{URL: "ws://pty.example.com/p", Token: "t"}, false, ""},
		{"http scheme", Interactive{URL: "https://pty.example.com/p", Token: "t"}, false, ""},
		{"user info", Interactive{URL: "wss://u:p@pty.example.com/p", Token: "t"}, false, ""}, //nolint:gosec // G101: a test URL that must be rejected
		{"no host", Interactive{URL: "wss:///p", Token: "t"}, false, ""},
		{"no token", Interactive{URL: "wss://pty.example.com/p"}, false, ""},
		{"garbage", Interactive{URL: "://", Token: "t"}, false, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseConsoleURL(tt.in)
			if (err == nil) != tt.wantOK {
				t.Fatalf("parseConsoleURL() error = %v, wantOK %v", err, tt.wantOK)
			}
			if tt.wantOK && (got.health != tt.health || !strings.Contains(got.ws, "token=t")) {
				t.Fatalf("parseConsoleURL() = %+v", got)
			}
		})
	}
}

func TestConsoleRejectsBadEndpoint(t *testing.T) {
	t.Parallel()
	p, _, pty := consoleBox(t)
	pty.srv.Close()
	err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{Command: []string{shellBash}, TTY: true})
	if err == nil || strings.Contains(err.Error(), ptyToken) {
		t.Fatalf("Exec() = %v", err)
	}
}

func TestKeepAliveExtendsSession(t *testing.T) {
	t.Parallel()
	c := &extendCounter{}
	s := &session{api: c, rec: Record{SessionID: "s"}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.keepAlive(ctx, 5*time.Millisecond); close(done) }()
	deadline := time.After(3 * time.Second)
	for c.count() < 2 {
		select {
		case <-deadline:
			t.Fatal("session was not extended")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	<-done
}

type extendCounter struct {
	apiClient
	mu sync.Mutex
	n  int
}

func (c *extendCounter) ExtendTimeout(context.Context, string, time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
	return errors.New("extension refused")
}

func (c *extendCounter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}
