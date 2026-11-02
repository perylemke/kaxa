package blaxel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/perylemke/kaxa/internal/sandbox"
)

const sandboxHost = "sbx-box-kaxa.us-pdx-1.bl.run"

func TestTerminalURL(t *testing.T) {
	t.Parallel()
	got, err := terminalURL("https://"+sandboxHost, "sek+ret/key", "kaxa-abcd", 30, 90, workspaceDir)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "wss" || u.Host != sandboxHost || u.Path != "/"+termPath {
		t.Fatalf("url = %s", u.Redacted())
	}
	q := u.Query()
	if q.Get("token") != "sek+ret/key" || q.Get("sessionId") != "kaxa-abcd" || q.Get("cols") != "90" || q.Get("rows") != "30" {
		t.Fatalf("query = %v", q)
	}
	if q.Get("workingDir") != workspaceDir {
		t.Fatalf("workingDir = %q", q.Get("workingDir"))
	}
	rejectTerminalURL(t)
}

func rejectTerminalURL(t *testing.T) {
	t.Helper()
	for _, bad := range []string{"http://sbx.bl.run", "https://example.com", "https://user:pw@sbx.bl.run"} {
		if _, err := terminalURL(bad, "k", "kaxa-abcd", 24, 80, blaxelHome); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if _, err := terminalURL("https://sbx.bl.run", "k", "../x", 24, 80, blaxelHome); err == nil {
		t.Fatal("accepted a bad session id")
	}
}

func TestTerminalDialRejectsForeignHost(t *testing.T) {
	t.Parallel()
	s := &sdkClient{apiKey: "test-key", termDial: func(context.Context, string, http.Header) (*websocket.Conn, *http.Response, error) {
		t.Fatal("dialed a foreign host")
		return nil, nil, nil
	}}
	err := s.attachTerminal(t.Context(), "https://example.com/sandbox", termSpec{dir: blaxelHome})
	if err == nil || !strings.Contains(err.Error(), "not a Blaxel host") {
		t.Fatalf("err = %v", err)
	}
}

func TestTerminalDialErrorRedactsToken(t *testing.T) {
	t.Parallel()
	const key = "sek+ret/key"
	s := &sdkClient{apiKey: key, termDial: func(_ context.Context, endpoint string, _ http.Header) (*websocket.Conn, *http.Response, error) {
		return nil, nil, errors.New("dial " + endpoint)
	}}
	err := s.attachTerminal(t.Context(), "https://"+sandboxHost, termSpec{dir: blaxelHome})
	if err == nil || strings.Contains(err.Error(), key) || strings.Contains(err.Error(), url.QueryEscape(key)) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "<redacted>") {
		t.Fatalf("err = %v", err)
	}
}

func TestTerminalBridgeExchangesFrames(t *testing.T) {
	t.Parallel()
	const key = "test-key"
	got := &termCapture{}
	srv := httptest.NewServer(http.HandlerFunc(got.serve))
	t.Cleanup(srv.Close)

	resize := make(chan sandbox.TermSize, 1)
	resize <- sandbox.TermSize{Rows: 40, Cols: 120}
	// Leave stdin open. EOF sends a close frame, and that frame can arrive
	// before the resize, which makes the server exit with no output.
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })
	go func() { _, _ = pw.Write([]byte("id\n")) }()
	s := &sdkClient{apiKey: key, workspace: "kaxa", termDial: got.dial(srv, key)}
	var stdout strings.Builder
	err := s.attachTerminal(t.Context(), "https://"+sandboxHost, termSpec{
		dir: blaxelHome, stdin: pr, stdout: &stdout, resize: resize,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "root\n" || !got.auth || !got.workspace {
		t.Fatalf("stdout = %q auth = %v workspace = %v", stdout.String(), got.auth, got.workspace)
	}
	got.mu.Lock()
	defer got.mu.Unlock()
	if got.input != "id\n" || len(got.resized) != 1 || got.resized[0] != (sandbox.TermSize{Rows: 40, Cols: 120}) {
		t.Fatalf("input = %q resized = %+v", got.input, got.resized)
	}
}

type termCapture struct {
	mu        sync.Mutex
	input     string
	resized   []sandbox.TermSize
	auth      bool
	workspace bool
}

func (c *termCapture) serve(w http.ResponseWriter, r *http.Request) {
	conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if c.ready(payload) {
			_ = conn.WriteJSON(termMessage{Type: termOutput, Data: "root\n"})
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
			drainTerm(conn)
			return
		}
	}
}

func (c *termCapture) ready(payload []byte) bool {
	var msg termMessage
	if json.Unmarshal(payload, &msg) != nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	switch msg.Type {
	case termInput:
		c.input += msg.Data
	case termResize:
		c.resized = append(c.resized, sandbox.TermSize{Rows: msg.Rows, Cols: msg.Cols})
	}
	return strings.Contains(c.input, "id\n") && len(c.resized) > 0
}

func (c *termCapture) dial(srv *httptest.Server, key string) wsDialFunc {
	return func(ctx context.Context, endpoint string, header http.Header) (*websocket.Conn, *http.Response, error) {
		if err := checkTermEndpoint(endpoint, key); err != nil {
			return nil, nil, err
		}
		c.auth = header.Get("Authorization") == "Bearer "+key && header.Get("X-Blaxel-Authorization") == "Bearer "+key
		c.workspace = header.Get("X-Blaxel-Workspace") == "kaxa"
		return dialServer(ctx, srv)
	}
}

func TestTerminalErrorFrame(t *testing.T) {
	t.Parallel()
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.WriteJSON(termMessage{Type: termError, Data: "boom"})
		time.Sleep(time.Second)
	}))
	t.Cleanup(srv.Close)
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })
	s := &sdkClient{apiKey: "test-key", termDial: dialLocal(srv)}
	var stderr strings.Builder
	err := s.attachTerminal(t.Context(), "https://"+sandboxHost, termSpec{
		dir: blaxelHome, stdin: pr, stderr: &stderr,
	})
	_ = pw.Close()
	if err == nil || !strings.Contains(err.Error(), "boom") || stderr.String() != "boom" {
		t.Fatalf("err = %v stderr = %q", err, stderr.String())
	}
}

func dialLocal(srv *httptest.Server) wsDialFunc {
	return func(ctx context.Context, endpoint string, _ http.Header) (*websocket.Conn, *http.Response, error) {
		if err := checkTermEndpoint(endpoint, "test-key"); err != nil {
			return nil, nil, err
		}
		return dialServer(ctx, srv)
	}
}

func checkTermEndpoint(endpoint, key string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	if u.Scheme != "wss" || u.Host != sandboxHost || u.Path != "/"+termPath {
		return errors.New("unexpected endpoint")
	}
	q := u.Query()
	if q.Get("token") != key || q.Get("workingDir") != blaxelHome || q.Get("cols") != "80" || q.Get("rows") != "24" {
		return errors.New("unexpected query")
	}
	if !strings.HasPrefix(q.Get("sessionId"), "kaxa-") {
		return errors.New("unexpected session")
	}
	return nil
}

func dialServer(ctx context.Context, srv *httptest.Server) (*websocket.Conn, *http.Response, error) {
	local, err := url.Parse(srv.URL)
	if err != nil {
		return nil, nil, err
	}
	local.Scheme = "ws"
	local.Path = "/" + termPath
	return (&websocket.Dialer{}).DialContext(ctx, local.String(), nil)
}

func drainTerm(conn *websocket.Conn) {
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
