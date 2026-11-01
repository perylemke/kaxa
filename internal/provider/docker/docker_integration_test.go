//go:build integration

package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/sandbox"
)

// These tests follow the flows of docker_live_test.go against a fake
// Docker Sandboxes API served over HTTP and WebSocket, so they exercise the
// real clients, cost nothing, and need no Docker account. Run the live tests
// to check against Docker.

type cloudBox struct {
	name, status, next string
	labels             map[string]string
	etag               int
	ports              []int
}

// fakeCloud is the management API, the sign-in, and the process endpoint of
// every sandbox, in one server.
type fakeCloud struct {
	srv   *httptest.Server
	mu    sync.Mutex
	boxes map[string]*cloudBox
	n     int
	cmds  [][]string
	stdin map[string]string
	snaps map[string]*cloudSnap
}

func newFakeCloud(t *testing.T) *fakeCloud {
	t.Helper()
	f := &fakeCloud{boxes: map[string]*cloudBox{}, stdin: map[string]string{}, snaps: map[string]*cloudSnap{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"hub-token"}`))
	})
	mux.HandleFunc("GET /v1/sandboxes", f.list)
	mux.HandleFunc("POST /v1/sandboxes", f.create)
	mux.HandleFunc("GET /v1/sandboxes/{id}", f.get)
	mux.HandleFunc("DELETE /v1/sandboxes/{id}", f.remove)
	mux.HandleFunc("POST /v1/sandboxes/{id}/{verb}", f.action)
	mux.HandleFunc("POST /v1/sandboxes/{id}/ports", f.publish)
	f.snapshotRoutes(mux)
	mux.HandleFunc("POST /ep/{id}/v1/processes", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Cmd []string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.cmds = append(f.cmds, body.Cmd)
		n := len(f.cmds)
		f.mu.Unlock()
		_, _ = fmt.Fprintf(w, `{"name":"processes/p%d"}`, n)
	})
	mux.HandleFunc("GET /ep/{id}/v1/processes/{proc}/interact", f.interact)
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeCloud) wire(id string, b *cloudBox) string {
	labels, _ := json.Marshal(b.labels)
	var ports []string
	for _, n := range b.ports {
		ports = append(ports, fmt.Sprintf(`{"sandboxPort":%d,"url":%q}`, n, portURL(n, id)))
	}
	return fmt.Sprintf(`{"name":"sandboxes/%s","displayName":%q,"core":{"status":%q,"etag":"\"%d\"","labels":%s,"ports":[%s],"endpoint":{"uri":"%s/ep/%s"}}}`,
		id, b.name, b.status, b.etag, labels, strings.Join(ports, ","), f.srv.URL, id)
}

func portURL(port int, id string) string {
	return fmt.Sprintf("https://%d-%s.sbx.example.test", port, id)
}

func (f *fakeCloud) publish(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Port struct{ Number int }
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	defer f.mu.Unlock()
	id := r.PathValue("id")
	b, ok := f.boxes[id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"notFound"}`))
		return
	}
	b.ports = append(b.ports, body.Port.Number)
	w.WriteHeader(http.StatusCreated)
	_, _ = fmt.Fprintf(w, `{"number":%d,"url":%q}`, body.Port.Number, portURL(body.Port.Number, id))
}

func (f *fakeCloud) list(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var items []string
	for id, b := range f.boxes {
		items = append(items, f.wire(id, b))
	}
	_, _ = fmt.Fprintf(w, `{"sandboxes":[%s]}`, strings.Join(items, ","))
}

func (f *fakeCloud) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DisplayName string
		Labels      map[string]string
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	id := fmt.Sprintf("s%d", f.n)
	b := &cloudBox{name: body.DisplayName, status: "starting", next: statusRunning, labels: body.Labels, etag: 1}
	f.boxes[id] = b
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(f.wire(id, b)))
}

func (f *fakeCloud) get(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := r.PathValue("id")
	b, ok := f.boxes[id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"notFound"}`))
		return
	}
	_, _ = w.Write([]byte(f.wire(id, b)))
	if b.next == "gone" {
		delete(f.boxes, id)
	} else if b.next != "" {
		b.status, b.next = b.next, ""
	}
}

func (f *fakeCloud) remove(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.boxes[r.PathValue("id")]
	if !ok || r.Header.Get("If-Match") == "" {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"notFound"}`))
		return
	}
	b.status, b.next = "deleting", "gone"
	w.WriteHeader(http.StatusAccepted)
}

func (f *fakeCloud) action(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := r.PathValue("id")
	b, ok := f.boxes[id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"notFound"}`))
		return
	}
	switch r.PathValue("verb") {
	case "start":
		b.status, b.next = "starting", statusRunning
	case "stop":
		b.status, b.next = "stopping", statusStopped
	case "renew-timeout":
	case "endpoint-credentials":
		_, _ = w.Write([]byte(`{"token":"endpoint-token"}`))
		return
	}
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(f.wire(id, b)))
}

func (f *fakeCloud) interact(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer endpoint-token" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	up := websocket.Upgrader{Subprotocols: []string{interactProtocol}}
	c, err := up.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = c.Close() }()
	var input strings.Builder
	for {
		var fr struct {
			Stdin      []byte `json:"stdin"`
			CloseStdin bool   `json:"closeStdin"`
		}
		if c.ReadJSON(&fr) != nil {
			return
		}
		input.Write(fr.Stdin)
		if !fr.CloseStdin {
			continue
		}
		f.mu.Lock()
		cmd := f.cmds[len(f.cmds)-1]
		if slices.Contains(cmd, "tar") {
			f.stdin["tar"] = input.String()
		}
		f.mu.Unlock()
		f.answer(c, cmd)
		return
	}
}

func (f *fakeCloud) answer(c *websocket.Conn, cmd []string) {
	switch {
	case slices.Contains(cmd, "pwd"):
		_ = c.WriteJSON(chunkFrame("stdout", "/home/agent/workspace\n"))
	case slices.Contains(cmd, "false"):
		_ = c.WriteJSON(map[string]any{"exited": map[string]any{"exitCode": 1}})
		return
	}
	_ = c.WriteJSON(map[string]any{"exited": map[string]any{"exitCode": 0}})
}

func newIntegrationProvider(t *testing.T) (*Provider, *fakeCloud) {
	t.Helper()
	f := newFakeCloud(t)
	hc := f.srv.Client()
	c := newRestClient("me", testPAT, hc)
	c.base = f.srv.URL
	c.auth.url = f.srv.URL + "/auth"
	c.restored = nil
	p := NewWithClients(c, &endpointClient{http: hc}, time.Millisecond, 5*time.Second)
	p.SetRestoredIndexPath(filepath.Join(t.TempDir(), restoredFile))
	c.restored = p.restoredLabels
	return p, f
}

func TestLifecycleFlow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p, f := newIntegrationProvider(t)

	id, err := p.Create(ctx, sandbox.Spec{Name: testSandbox, Labels: []string{labelKaxa, sandbox.SyncLabel}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := p.Create(ctx, sandbox.Spec{Name: testSandbox}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate create err = %v", err)
	}
	infos, err := p.List(ctx)
	if err != nil || len(infos) != 1 || infos[0].ID != id || infos[0].Status != statusRunning {
		t.Fatalf("list = %+v err = %v", infos, err)
	}

	if err := p.SyncWorkspace(ctx, id, strings.NewReader("TARBALL")); err != nil {
		t.Fatalf("sync: %v", err)
	}
	f.mu.Lock()
	extracted := f.stdin["tar"]
	f.mu.Unlock()
	if extracted != "TARBALL" {
		t.Fatalf("extracted = %q", extracted)
	}
	steps := []project.Step{{Kind: project.KindGo, Commands: [][]string{{"go", "build", "./..."}}}}
	if err := p.SetupProjects(ctx, id, "demo", steps); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if ok, err := p.RepoExists(ctx, id, "demo"); err != nil || !ok {
		t.Fatalf("repo exists = %v err = %v", ok, err)
	}

	var out strings.Builder
	if err := p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{"pwd"}, Stdout: &out}); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if out.String() != "/home/agent/workspace\n" {
		t.Fatalf("exec output = %q", out.String())
	}
	err = p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{"false"}})
	if exitErr, ok := errors.AsType[*sandbox.ExitError](err); !ok || exitErr.Code != 1 {
		t.Fatalf("exec false err = %v", err)
	}

	if err := p.Restart(ctx, id); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if err := p.Destroy(ctx, id); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if _, err := p.Inspect(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("inspect after destroy err = %v", err)
	}
}

func TestCreateWithAPIPublishesThePort(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p, f := newIntegrationProvider(t)

	req := lifecycle.Request{Name: testSandbox, API: true, APIPort: 8000, APICmd: []string{"uvicorn", "app.main:app"}}
	res, err := lifecycle.Create(ctx, p, req, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	detail, err := p.Inspect(ctx, res.ID)
	if err != nil || detail.URL != "https://8000-s1.sbx.example.test" {
		t.Fatalf("detail = %+v err = %v", detail, err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.cmds) < 2 || !slices.Contains(f.cmds[0], "uvicorn") || f.cmds[1][0] != "bash" {
		t.Fatalf("commands = %v, want the service and then the probe", f.cmds)
	}
}

func TestSandboxesOfOthersAreIgnored(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p, f := newIntegrationProvider(t)
	f.mu.Lock()
	f.boxes["theirs"] = &cloudBox{name: "theirs", status: statusRunning, labels: map[string]string{}, etag: 1}
	f.mu.Unlock()

	if infos, err := p.List(ctx); err != nil || len(infos) != 0 {
		t.Fatalf("list = %+v err = %v", infos, err)
	}
	if err := p.Destroy(ctx, "theirs"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("destroy err = %v, want ErrNotFound", err)
	}
	if err := p.Exec(ctx, "theirs", sandbox.ExecOptions{Command: []string{"ls"}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("exec err = %v, want ErrNotFound", err)
	}
}

func TestCreateCheckRejectsUnsupportedFeatures(t *testing.T) {
	t.Parallel()
	p := New()
	for name, req := range map[string]lifecycle.Request{
		statusRunning: {Name: testSandbox, Running: true},
		"public":      {Name: testSandbox, Public: true},
	} {
		if err := lifecycle.Check(p, req); !errors.Is(err, sandbox.ErrUnsupported) {
			t.Errorf("%s: err = %v, want ErrUnsupported", name, err)
		}
	}
	if err := lifecycle.Check(p, lifecycle.Request{Name: testSandbox, Template: "t:1"}); err != nil {
		t.Errorf("template must be accepted: %v", err)
	}
}
