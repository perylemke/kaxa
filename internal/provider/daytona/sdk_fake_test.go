package daytona

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
)

const fakeKey = "fake-api-key"

const (
	stateStarted = "started"
	msgKey       = "message"
	msgBoom      = "boom"
	cmdID1       = "cmd-1"
	echoCmd      = "echo"
	keyStatus    = "status"
	providerID   = "daytona"
)

// fakeServer is a Daytona REST API and toolbox over httptest, for the SDK
// adapters. override answers first; a false third result falls to the defaults.
type fakeServer struct {
	t   *testing.T
	srv *httptest.Server

	mu       sync.Mutex
	requests []string
	bodies   map[string]string
	// state is the sandbox state every sandbox answer reports.
	state  string
	labels map[string]string
	// override answers a request before the defaults do.
	override func(r *http.Request) (status int, body any, ok bool)
	// logs are the session command logs; exit is the exit code, nil while running.
	stdout, stderr string
	exit           any
	// ptyHandler serves the terminal websocket.
	ptyHandler func(c *websocket.Conn)
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	f := &fakeServer{
		t: t, state: stateStarted, bodies: map[string]string{},
		labels: map[string]string{labelManaged: labelTrue, labelName: boxName},
		exit:   float64(0),
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

// client returns an sdkClient pointed at the fake.
func (f *fakeServer) client() *sdkClient {
	f.t.Helper()
	c, err := newSDKClientAt(fakeKey, f.srv.URL+"/api")
	if err != nil {
		f.t.Fatalf("newSDKClientAt: %v", err)
	}
	return c
}

func (f *fakeServer) toolbox() *sdkToolbox { return &sdkToolbox{c: f.client()} }

// seen reports whether a request line "METHOD path" was made.
func (f *fakeServer) seen(line string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.requests {
		if strings.Contains(r, line) {
			return true
		}
	}
	return false
}

func (f *fakeServer) body(key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bodies[key]
}

func (f *fakeServer) sandboxJSON() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return map[string]any{
		"id": sandboxID, "organizationId": "o", "name": boxName, "user": providerID,
		"env": map[string]string{}, "labels": f.labels, "public": false, "networkBlockAll": false,
		"kvm": false, "target": "us", "cpu": 2, "gpu": 0, "memory": 4, "disk": 10,
		"state": f.state, "toolboxProxyUrl": f.srv.URL + "/tb",
		"createdAt": "2026-01-02T03:04:05Z",
	}
}

func snapshotJSON(name, state string) map[string]any {
	return map[string]any{
		"id": "snap-" + name, "general": false, "name": name, "state": state, "size": 1.5,
		"entrypoint": []string{}, "cpu": 1, "gpu": 0, "mem": 2, "disk": 3,
		"errorReason": nil, "createdAt": "2026-01-02T03:04:05Z", "updatedAt": "2026-01-02T03:04:05Z",
		"lastUsedAt": nil, "sourceSandboxId": nil,
	}
}

func (f *fakeServer) serve(w http.ResponseWriter, r *http.Request) {
	data, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	f.bodies[r.Method+" "+r.URL.Path] = string(data)
	override := f.override
	f.mu.Unlock()
	if strings.HasSuffix(r.URL.Path, "/process/pty/create-connect") {
		f.servePty(w, r)
		return
	}
	if override != nil {
		if s, b, ok := override(r); ok {
			writeJSON(w, s, b)
			return
		}
	}
	writeJSON(w, http.StatusOK, f.defaults(r))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (f *fakeServer) defaults(r *http.Request) any {
	p := strings.TrimPrefix(r.URL.Path, "/api")
	switch {
	case strings.HasPrefix(r.URL.Path, "/tb/"):
		return f.toolboxDefaults(r)
	case p == "/sandbox" && r.Method == http.MethodGet:
		return map[string]any{"items": []any{f.sandboxJSON()}, "nextCursor": nil}
	case strings.HasSuffix(p, "/preview-url") || strings.Contains(p, "/ports/"):
		return map[string]any{"sandboxId": sandboxID, "url": "https://80-box.example", "token": "t"}
	case p == "/snapshots" && r.Method == http.MethodGet:
		return map[string]any{
			"items": []any{snapshotJSON("kaxa-a", "active"), snapshotJSON("other", "active")},
			"total": 2, "page": 1, "totalPages": 1,
		}
	case strings.HasPrefix(p, "/snapshots/"):
		return snapshotJSON(strings.TrimPrefix(p, "/snapshots/"), "active")
	}
	return f.sandboxJSON()
}

func (f *fakeServer) toolboxDefaults(r *http.Request) any {
	p := r.URL.Path
	switch {
	case strings.HasSuffix(p, "/logs"):
		return map[string]any{"stdout": f.stdout, "stderr": f.stderr, "output": f.stdout + f.stderr}
	case strings.Contains(p, "/command/"):
		out := map[string]any{"id": cmdID1, "command": "x"}
		if f.exit != nil {
			out["exitCode"] = f.exit
		}
		return out
	case strings.HasSuffix(p, "/exec"):
		return map[string]any{"cmdId": cmdID1}
	}
	return map[string]any{}
}

var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

func (f *fakeServer) servePty(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = c.Close() }()
	f.mu.Lock()
	h := f.ptyHandler
	f.mu.Unlock()
	if h != nil {
		h(c)
	}
}

// errBody is the JSON error body of the Daytona API.
func errBody(status int, message string) map[string]any {
	return map[string]any{"message": message, "statusCode": status}
}
