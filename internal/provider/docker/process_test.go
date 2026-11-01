package docker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
)

// endpointServer stands in for the process API of a sandbox. For each process
// it reads the attach frame, then input until closeStdin, and answers with
// reply.
type endpointServer struct {
	srv      *httptest.Server
	mu       sync.Mutex
	created  map[string]any
	attached string
	stdin    strings.Builder
	proto    string
	auth     string
	resized  []terminalSize
}

func newEndpointServer(t *testing.T, reply func(c *websocket.Conn)) *endpointServer {
	t.Helper()
	s := &endpointServer{}
	up := websocket.Upgrader{Subprotocols: []string{interactProtocol}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /sbx/v1/processes", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&s.created)
		s.mu.Unlock()
		_, _ = w.Write([]byte(`{"name":"processes/p1"}`))
	})
	mux.HandleFunc("GET /sbx/v1/processes/p1/interact", func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		s.mu.Lock()
		s.proto = c.Subprotocol()
		s.mu.Unlock()
		s.serve(c, reply)
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	return s
}

func (s *endpointServer) serve(c *websocket.Conn, reply func(c *websocket.Conn)) {
	for {
		var f struct {
			Attach     *struct{ Name string } `json:"attach"`
			Stdin      []byte                 `json:"stdin"`
			CloseStdin bool                   `json:"closeStdin"`
			Resize     *terminalSize          `json:"resize"`
		}
		if err := c.ReadJSON(&f); err != nil {
			return
		}
		s.mu.Lock()
		switch {
		case f.Attach != nil:
			s.attached = f.Attach.Name
		case f.Resize != nil:
			s.resized = append(s.resized, *f.Resize)
		default:
			s.stdin.Write(f.Stdin)
		}
		s.mu.Unlock()
		if f.CloseStdin {
			reply(c)
			return
		}
	}
}

func (s *endpointServer) endpoint() Endpoint { return Endpoint{URI: s.srv.URL + "/sbx"} }

func chunkFrame(stream, data string) map[string]any {
	return map[string]any{"chunk": map[string]any{"stream": stream, "data": []byte(data), "streamSequence": 1}}
}

func TestStartRunsAndStreamsAProcess(t *testing.T) {
	t.Parallel()
	srv := newEndpointServer(t, func(c *websocket.Conn) {
		_ = c.WriteJSON(map[string]any{"heartbeat": map[string]any{}})
		_ = c.WriteJSON(chunkFrame("stdout", "hello"))
		_ = c.WriteJSON(chunkFrame("stderr", "warn"))
		_ = c.WriteJSON(map[string]any{"exited": map[string]any{"exitCode": 7}})
	})
	c := &endpointClient{http: srv.srv.Client()}
	proc, err := c.Start(context.Background(), srv.endpoint(), Credential{Token: testToken}, ProcessSpec{
		Argv: []string{cmdCat}, Dir: "/w", User: "agent", Env: map[string]string{"A": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = proc.Close() }()
	if err := proc.WriteStdin([]byte("in")); err != nil {
		t.Fatal(err)
	}
	if err := proc.CloseStdin(); err != nil {
		t.Fatal(err)
	}
	events := readUntilExit(t, proc)
	checkEvents(t, events)
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.auth != "Bearer "+testToken || srv.proto != interactProtocol || srv.attached != "processes/p1" || srv.stdin.String() != "in" {
		t.Fatalf("auth=%q proto=%q attached=%q stdin=%q", srv.auth, srv.proto, srv.attached, srv.stdin.String())
	}
	checkCreateBody(t, srv.created)
}

func checkEvents(t *testing.T, events []Event) {
	t.Helper()
	if len(events) != 3 || string(events[0].Data) != "hello" || events[0].Stderr ||
		string(events[1].Data) != "warn" || !events[1].Stderr || events[2].ExitCode != 7 {
		t.Fatalf("events = %+v", events)
	}
}

func readUntilExit(t *testing.T, proc Process) []Event {
	t.Helper()
	var events []Event
	for {
		ev, err := proc.Next()
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, ev)
		if ev.Exited {
			return events
		}
	}
}

func checkCreateBody(t *testing.T, created map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(created)
	for _, want := range []string{`"cmd":["cat"]`, `"workingDir":"/w"`, `"user":"agent"`, `"env":{"A":"1"}`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("create body lacks %s: %s", want, raw)
		}
	}
	if strings.Contains(string(raw), "pty") {
		t.Errorf("create body asks for a terminal: %s", raw)
	}
}

func TestStartTerminalAndResize(t *testing.T) {
	t.Parallel()
	srv := newEndpointServer(t, func(c *websocket.Conn) {
		_ = c.WriteJSON(map[string]any{"exited": map[string]any{}})
	})
	c := &endpointClient{http: srv.srv.Client()}
	proc, err := c.Start(context.Background(), srv.endpoint(), Credential{Token: testToken}, ProcessSpec{
		Argv: []string{"bash"}, TTY: true, Rows: 30, Cols: 90,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = proc.Close() }()
	if err := proc.Resize(40, 100); err != nil {
		t.Fatal(err)
	}
	if err := proc.CloseStdin(); err != nil {
		t.Fatal(err)
	}
	if ev, err := proc.Next(); err != nil || !ev.Exited {
		t.Fatalf("ev = %+v, err = %v", ev, err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	pty, _ := srv.created["pty"].(map[string]any)
	size, _ := pty["initialSize"].(map[string]any)
	if size["rows"] != float64(30) || size["cols"] != float64(90) {
		t.Fatalf("create body = %v", srv.created)
	}
	if len(srv.resized) != 1 || srv.resized[0] != (terminalSize{Rows: 40, Cols: 100}) {
		t.Fatalf("resized = %v", srv.resized)
	}
}

func TestStartLargeInputIsSplit(t *testing.T) {
	t.Parallel()
	big := strings.Repeat("x", 3*stdinChunk+5)
	srv := newEndpointServer(t, func(c *websocket.Conn) {
		_ = c.WriteJSON(map[string]any{"exited": map[string]any{}})
	})
	c := &endpointClient{http: srv.srv.Client()}
	proc, err := c.Start(context.Background(), srv.endpoint(), Credential{Token: testToken}, ProcessSpec{Argv: []string{cmdCat}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = proc.Close() }()
	if err := proc.WriteStdin([]byte(big)); err != nil {
		t.Fatal(err)
	}
	_ = proc.CloseStdin()
	_, _ = proc.Next()
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.stdin.String() != big {
		t.Fatalf("server read %d bytes, want %d", srv.stdin.Len(), len(big))
	}
}

func TestNextReportsStreamErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		reply   func(c *websocket.Conn)
		wantErr string
	}{
		{"error frame", func(c *websocket.Conn) {
			_ = c.WriteJSON(map[string]any{"code": "unavailable", "message": "sandbox stopped"})
		}, "unavailable: sandbox stopped"},
		{"closed before exit", func(c *websocket.Conn) {
			_ = c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		}, "closed before the process exited"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := newEndpointServer(t, tt.reply)
			c := &endpointClient{http: srv.srv.Client()}
			proc, err := c.Start(context.Background(), srv.endpoint(), Credential{Token: testToken}, ProcessSpec{Argv: []string{"x"}})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = proc.Close() }()
			_ = proc.CloseStdin()
			if _, err := proc.Next(); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestStartRefusedByTheAPI(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"unauthenticated"}`))
	}))
	defer srv.Close()
	c := &endpointClient{http: srv.Client()}
	_, err := c.Start(context.Background(), Endpoint{URI: srv.URL}, Credential{Token: testToken}, ProcessSpec{Argv: []string{"x"}})
	if apiErr, ok := errors.AsType[*APIError](err); !ok || apiErr.Code != codeUnauthenticated {
		t.Fatalf("err = %v", err)
	}
}

func TestStartRejectsBadProcessName(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"name":"elsewhere/p1"}`))
	}))
	defer srv.Close()
	c := &endpointClient{http: srv.Client()}
	if _, err := c.Start(context.Background(), Endpoint{URI: srv.URL}, Credential{Token: testToken}, ProcessSpec{Argv: []string{"x"}}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestStartRefusesInsecureEndpoint(t *testing.T) {
	t.Parallel()
	c := &endpointClient{http: http.DefaultClient}
	for _, uri := range []string{"http://sandbox.example.com/x", "ftp://sandbox.example.com", "https://", "::bad", ""} {
		if _, err := c.Start(context.Background(), Endpoint{URI: uri}, Credential{Token: testToken}, ProcessSpec{Argv: []string{"x"}}); err == nil {
			t.Errorf("endpoint %q must be refused", uri)
		}
	}
}

func TestCheckEndpointAcceptsHTTPSAndLoopback(t *testing.T) {
	t.Parallel()
	for _, uri := range []string{"https://sbx.example.test/abc", "http://127.0.0.1:8080/x", "http://[::1]:80"} {
		if _, err := checkEndpoint(uri); err != nil {
			t.Errorf("checkEndpoint(%q) = %v", uri, err)
		}
	}
}

func TestEndpointURLKeepsPathPrefix(t *testing.T) {
	t.Parallel()
	u, _ := url.Parse("https://sbx.example.test/abc/?x=1#f")
	if got := endpointURL(u, "/v1/processes"); got != "https://sbx.example.test/abc/v1/processes" {
		t.Fatalf("got %q", got)
	}
}

func TestSizeOrDefault(t *testing.T) {
	t.Parallel()
	if got := sizeOrDefault(0, 0); got != (terminalSize{Rows: defaultRows, Cols: defaultCols}) {
		t.Fatalf("got %+v", got)
	}
	if got := sizeOrDefault(10, 20); got != (terminalSize{Rows: 10, Cols: 20}) {
		t.Fatalf("got %+v", got)
	}
}
