package vercel

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
)

const (
	testToken = "vcp_secret_token"
	boxName   = "box"
)

type fakeSandbox struct {
	name     string
	tags     map[string]string
	status   string
	session  string
	snapshot string
	image    string
	vcpus    int
	ports    []int
	env      map[string]any
	expires  time.Time
	created  time.Time
	lag      int
}

const methodAutomatic = "automatic"

type fakeSnap struct {
	id, sandbox, status, method string
	created                     time.Time
}

// fakeCmd is one command the fake runs. exec decides its output.
type fakeCmd struct {
	id     string
	body   map[string]any
	stdin  bytes.Buffer
	closed chan struct{}
	killed chan struct{}
}

// fakeResult is what a command prints and how it ends.
type fakeResult struct {
	stdout, stderr string
	code           int
	noExit         bool
	failure        string
	cut            bool
	block          bool
}

// fakeVercel is a Vercel Sandbox API over httptest.
type fakeVercel struct {
	t         *testing.T
	mu        sync.Mutex
	srv       *httptest.Server
	sandboxes map[string]*fakeSandbox
	snaps     []*fakeSnap
	cmds      map[string]*fakeCmd
	seq       int
	pageSize  int
	// failures maps "METHOD /path" patterns to a status that replaces the answer.
	failures map[string]int
	queries  []url.Values
	bodies   map[string][]map[string]any
	exec     func(c *fakeCmd) fakeResult
	tar      []byte
	tarCwd   string
	mkdirs   []string
	killedID chan string
	// lagStop is how many reads still report "stopping" after a stop.
	lagStop int
	// pty is the terminal server the interactive endpoint points at.
	pty *fakePTY
}

func newFake(t *testing.T) *fakeVercel {
	t.Helper()
	f := &fakeVercel{
		t: t, sandboxes: map[string]*fakeSandbox{}, cmds: map[string]*fakeCmd{},
		failures: map[string]int{}, bodies: map[string][]map[string]any{},
		killedID: make(chan string, 8),
		exec:     func(*fakeCmd) fakeResult { return fakeResult{} },
	}
	mux := http.NewServeMux()
	f.route(mux, "POST /v4/sandboxes", f.create)
	f.route(mux, "GET /v2/sandboxes", f.list)
	f.route(mux, "GET /v2/sandboxes/snapshots", f.listSnaps)
	f.route(mux, "DELETE /v2/sandboxes/snapshots/{id}", f.deleteSnap)
	f.route(mux, "GET /v2/sandboxes/{name}", f.get)
	f.route(mux, "PATCH /v2/sandboxes/{name}", f.patch)
	f.route(mux, "DELETE /v2/sandboxes/{name}", f.remove)
	f.route(mux, "POST /v2/sandboxes/sessions/{sid}/interactive", f.interactive)
	f.route(mux, "POST /v2/sandboxes/sessions/{sid}/stop", f.stop)
	f.route(mux, "POST /v2/sandboxes/sessions/{sid}/extend-timeout", f.extend)
	f.route(mux, "POST /v2/sandboxes/sessions/{sid}/fs/mkdir", f.mkdir)
	f.route(mux, "POST /v2/sandboxes/sessions/{sid}/fs/write", f.write)
	f.route(mux, "POST /v2/sandboxes/sessions/{sid}/cmd", f.command)
	f.route(mux, "POST /v2/sandboxes/sessions/{sid}/cmd/{cid}/stdin", f.stdin)
	f.route(mux, "POST /v2/sandboxes/sessions/{sid}/cmd/{cid}/kill", f.kill)
	f.route(mux, "POST /v3/sandboxes/sessions/{sid}/snapshot", f.snapshot)
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// route registers a handler behind the bearer check, the failure table, and
// the request log. The pattern is also the key in failures and bodies.
func (f *fakeVercel) route(mux *http.ServeMux, pattern string, fn http.HandlerFunc) {
	mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			f.fail(w, http.StatusUnauthorized, "bad token")
			return
		}
		// The real API refuses a JSON POST that carries no JSON content type.
		if r.Method == http.MethodPost && !strings.HasSuffix(r.URL.Path, "/fs/write") &&
			!strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			f.fail(w, http.StatusUnsupportedMediaType, "Unsupported Media Type")
			return
		}
		f.mu.Lock()
		f.queries = append(f.queries, r.URL.Query())
		status := f.failures[pattern]
		f.mu.Unlock()
		if status != 0 {
			f.fail(w, status, "forced failure")
			return
		}
		if r.Header.Get("Content-Type") == "application/json" {
			raw, _ := io.ReadAll(r.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			f.mu.Lock()
			f.bodies[pattern] = append(f.bodies[pattern], body)
			f.mu.Unlock()
			r.Body = io.NopCloser(bytes.NewReader(raw))
		}
		fn(w, r)
	})
}

func (f *fakeVercel) fail(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "x", "message": msg}})
}

func (f *fakeVercel) reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func decode(r *http.Request) map[string]any {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	return body
}

func (f *fakeVercel) nextID(prefix string) string {
	f.seq++
	return prefix + "-" + strconv.Itoa(f.seq)
}

// body returns the n-th recorded JSON body of a route.
func (f *fakeVercel) body(pattern string, n int) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n >= len(f.bodies[pattern]) {
		f.t.Fatalf("no body #%d for %s", n, pattern)
	}
	return f.bodies[pattern][n]
}

func (f *fakeVercel) calls(pattern string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.bodies[pattern])
}

func (f *fakeVercel) lastQuery() url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.queries[len(f.queries)-1]
}

func (f *fakeVercel) fail2(pattern string, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures[pattern] = status
}

func (f *fakeVercel) wire(sb *fakeSandbox) map[string]any {
	s := map[string]any{
		keyName: sb.name, "status": sb.status, "currentSessionId": sb.session,
		"currentSnapshotId": sb.snapshot, "image": sb.image, "region": "iad1", "runtime": "node22",
		"vcpus": sb.vcpus, "memory": sb.vcpus * 2048, "createdAt": sb.created.UnixMilli(), "tags": sb.tags,
	}
	if sb.status == statusRunning {
		s["expiresAt"] = sb.expires.UnixMilli()
	}
	return s
}

func (f *fakeVercel) envelope(sb *fakeSandbox) map[string]any {
	routes := []map[string]any{}
	for _, p := range sb.ports {
		routes = append(routes, map[string]any{"port": p, "url": fmt.Sprintf("https://%s-%d.vercel.run", sb.name, p)})
	}
	return map[string]any{"sandbox": f.wire(sb), "session": map[string]string{"id": sb.session}, "routes": routes}
}

func (f *fakeVercel) startSession(sb *fakeSandbox) {
	sb.session = f.nextID("sess")
	sb.status = statusRunning
	sb.expires = time.Now().Add(sessionTimeout)
}

func (f *fakeVercel) create(w http.ResponseWriter, r *http.Request) {
	body := decode(r)
	f.mu.Lock()
	defer f.mu.Unlock()
	name, _ := body["name"].(string)
	if _, taken := f.sandboxes[name]; taken {
		f.fail(w, http.StatusConflict, "name taken")
		return
	}
	sb := &fakeSandbox{name: name, tags: map[string]string{}, created: time.Now(), vcpus: 2, image: "vercel/sandbox/universal"}
	if tags, ok := body["tags"].(map[string]any); ok {
		for k, v := range tags {
			sb.tags[k], _ = v.(string)
		}
	}
	if img, ok := body["image"].(string); ok {
		sb.image = img
	}
	if res, ok := body["resources"].(map[string]any); ok {
		sb.vcpus = int(asFloat(res["vcpus"]))
	}
	for _, p := range anyList(body["ports"]) {
		sb.ports = append(sb.ports, int(asFloat(p)))
	}
	sb.env, _ = body["env"].(map[string]any)
	f.startSession(sb)
	f.sandboxes[name] = sb
	f.reply(w, f.envelope(sb))
}

func anyList(v any) []any {
	l, _ := v.([]any)
	return l
}

func (f *fakeVercel) list(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var names []string
	for n := range f.sandboxes {
		names = append(names, n)
	}
	sortStrings(names)
	start := 0
	if c := r.URL.Query().Get("cursor"); c != "" {
		start, _ = strconv.Atoi(c)
	}
	end := len(names)
	var next any
	if f.pageSize > 0 && start+f.pageSize < end {
		end = start + f.pageSize
		next = strconv.Itoa(end)
	}
	out := []map[string]any{}
	for _, n := range names[start:end] {
		out = append(out, f.wire(f.sandboxes[n]))
	}
	f.reply(w, map[string]any{"pagination": map[string]any{"count": len(out), "next": next}, "sandboxes": out})
}

func sortStrings(s []string) {
	for i := range s {
		for j := i + 1; j < len(s); j++ {
			if s[j] < s[i] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}

func (f *fakeVercel) get(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sb, ok := f.sandboxes[r.PathValue("name")]
	if !ok {
		f.fail(w, http.StatusNotFound, "no such sandbox")
		return
	}
	if r.URL.Query().Get("resume") == tagTrue {
		if sb.status == statusStopped {
			f.startSession(sb)
		}
	} else if sb.status == statusStopping {
		if sb.lag > 0 {
			sb.lag--
		} else {
			sb.status = statusStopped
		}
	}
	f.reply(w, f.envelope(sb))
}

func (f *fakeVercel) patch(w http.ResponseWriter, r *http.Request) {
	body := decode(r)
	f.mu.Lock()
	defer f.mu.Unlock()
	sb, ok := f.sandboxes[r.PathValue("name")]
	if !ok {
		f.fail(w, http.StatusNotFound, "no such sandbox")
		return
	}
	if ports, ok := body["ports"]; ok {
		sb.ports = nil
		for _, p := range anyList(ports) {
			sb.ports = append(sb.ports, int(asFloat(p)))
		}
	}
	if tags, ok := body["tags"].(map[string]any); ok {
		sb.tags = map[string]string{}
		for k, v := range tags {
			sb.tags[k], _ = v.(string)
		}
	}
	if id, ok := body["currentSnapshotId"].(string); ok {
		sb.snapshot = id
	}
	f.reply(w, map[string]any{})
}

func (f *fakeVercel) remove(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	name := r.PathValue("name")
	if _, ok := f.sandboxes[name]; !ok {
		f.fail(w, http.StatusNotFound, "no such sandbox")
		return
	}
	delete(f.sandboxes, name)
	f.reply(w, map[string]any{})
}

func (f *fakeVercel) bySession(sid string) *fakeSandbox {
	for _, sb := range f.sandboxes {
		if sb.session == sid {
			return sb
		}
	}
	return nil
}

func (f *fakeVercel) stopLocked(sb *fakeSandbox) {
	f.snaps = append(f.snaps, &fakeSnap{id: f.nextID("snap"), sandbox: sb.name, status: snapshotCreated, method: methodAutomatic, created: time.Now()})
	sb.status = statusStopping
	sb.lag = f.lagStop
	if sb.lag == 0 {
		sb.status = statusStopped
	}
}

func (f *fakeVercel) stop(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sb := f.bySession(r.PathValue("sid"))
	if sb == nil {
		f.fail(w, http.StatusNotFound, "no such session")
		return
	}
	f.stopLocked(sb)
	f.reply(w, map[string]any{})
}

func (f *fakeVercel) extend(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sb := f.bySession(r.PathValue("sid"))
	if sb == nil {
		f.fail(w, http.StatusNotFound, "no such session")
		return
	}
	sb.expires = time.Now().Add(sessionTimeout)
	f.reply(w, map[string]any{})
}

func (f *fakeVercel) mkdir(w http.ResponseWriter, r *http.Request) {
	body := decode(r)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mkdirs = append(f.mkdirs, asStr(body["path"]))
	f.reply(w, map[string]any{})
}

func (f *fakeVercel) write(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tar, f.tarCwd = raw, r.Header.Get("x-cwd")
	if r.Header.Get("Content-Type") != "application/gzip" {
		f.fail(w, http.StatusBadRequest, "bad content type")
		return
	}
	f.reply(w, map[string]any{})
}

func item(kind int, payload any) []byte {
	raw, err := cbor.Marshal([]any{kind, payload})
	if err != nil {
		panic(err)
	}
	return raw
}

//nolint:gocyclo // one scenario that checks many fields of a single response
func (f *fakeVercel) command(w http.ResponseWriter, r *http.Request) {
	body := decode(r)
	f.mu.Lock()
	c := &fakeCmd{id: f.nextID("cmd"), body: body, closed: make(chan struct{}), killed: make(chan struct{})}
	f.cmds[c.id] = c
	f.mu.Unlock()
	if wait, _ := body["wait"].(bool); !wait {
		f.reply(w, map[string]any{"command": map[string]string{"id": c.id}})
		return
	}
	w.Header().Set("Content-Type", cborStreamType)
	_, _ = w.Write(item(itemCommand, map[string]any{"id": c.id}))
	if fl, ok := w.(http.Flusher); ok {
		fl.Flush()
	}
	if attach, _ := body["attachStdin"].(bool); attach {
		select {
		case <-c.closed:
		case <-r.Context().Done():
			return
		case <-time.After(10 * time.Second):
			f.t.Error("fake: stdin was never closed")
			return
		}
	}
	res := f.exec(c)
	if res.block {
		select {
		case <-c.killed:
		case <-r.Context().Done():
		}
		return
	}
	if res.stdout != "" {
		_, _ = w.Write(item(itemStdout, []byte(res.stdout)))
	}
	if res.stderr != "" {
		_, _ = w.Write(item(itemStderr, []byte(res.stderr)))
	}
	switch {
	case res.cut:
	case res.failure != "":
		_, _ = w.Write(item(itemFailure, map[string]any{"code": "boom", "message": res.failure}))
	case res.noExit:
		_, _ = w.Write(item(itemCommand, map[string]any{"id": c.id}))
	default:
		_, _ = w.Write(item(itemCommand, map[string]any{"id": c.id, "exitCode": res.code}))
	}
}

func (f *fakeVercel) stdin(w http.ResponseWriter, r *http.Request) {
	body := decode(r)
	f.mu.Lock()
	c := f.cmds[r.PathValue("cid")]
	f.mu.Unlock()
	if c == nil {
		f.fail(w, http.StatusNotFound, "no such command")
		return
	}
	data, _ := base64.StdEncoding.DecodeString(asStr(body["data"]))
	f.mu.Lock()
	c.stdin.Write(data)
	f.mu.Unlock()
	if closing, _ := body["close"].(bool); closing {
		close(c.closed)
	}
	f.reply(w, map[string]any{"bytesWritten": len(data)})
}

func (f *fakeVercel) kill(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	c := f.cmds[r.PathValue("cid")]
	f.mu.Unlock()
	if c == nil {
		f.fail(w, http.StatusNotFound, "no such command")
		return
	}
	close(c.killed)
	f.killedID <- c.id
	f.reply(w, map[string]any{})
}

func (f *fakeVercel) snapshot(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sb := f.bySession(r.PathValue("sid"))
	if sb == nil {
		f.fail(w, http.StatusNotFound, "no such session")
		return
	}
	s := &fakeSnap{id: f.nextID("snap"), sandbox: sb.name, status: snapshotCreated, method: snapshotManual, created: time.Now()}
	f.snaps = append(f.snaps, s)
	sb.status = statusStopped
	f.reply(w, map[string]any{"snapshot": snapWire(s)})
}

func snapWire(s *fakeSnap) map[string]any {
	return map[string]any{"id": s.id, "status": s.status, "creationMethod": s.method, "createdAt": s.created.UnixMilli()}
}

func (f *fakeVercel) listSnaps(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	name := r.URL.Query().Get("name")
	var out []map[string]any
	for _, s := range f.snaps {
		if name == "" || s.sandbox == name {
			out = append(out, snapWire(s))
		}
	}
	start := 0
	if c := r.URL.Query().Get("cursor"); c != "" {
		start, _ = strconv.Atoi(c)
	}
	end := len(out)
	var next any
	if f.pageSize > 0 && start+f.pageSize < end {
		end = start + f.pageSize
		next = strconv.Itoa(end)
	}
	if out == nil {
		out = []map[string]any{}
	}
	f.reply(w, map[string]any{"pagination": map[string]any{"count": end - start, "next": next}, "snapshots": out[start:end]})
}

func (f *fakeVercel) deleteSnap(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, s := range f.snaps {
		if s.id == r.PathValue("id") {
			f.snaps = append(f.snaps[:i], f.snaps[i+1:]...)
			f.reply(w, map[string]any{})
			return
		}
	}
	f.fail(w, http.StatusNotFound, "no such snapshot")
}

// newTestProvider returns a provider wired to a fresh fake.
func newTestProvider(t *testing.T) (*Provider, *fakeVercel) {
	t.Helper()
	f := newFake(t)
	p, err := NewWithBaseURL(f.srv.URL, testToken, "proj", "")
	if err != nil {
		t.Fatal(err)
	}
	p.SetSnapshotIndexPath(filepath.Join(t.TempDir(), "idx.json"))
	p.SetReadiness(2*time.Second, time.Millisecond)
	return p, f
}

func asMap(v any) map[string]any { m, _ := v.(map[string]any); return m }

func asList(v any) []any { l, _ := v.([]any); return l }

func asFloat(v any) float64 { f, _ := v.(float64); return f }

func asStr(v any) string { s, _ := v.(string); return s }

const (
	keyName    = "name"
	keyCode    = "code"
	keyMessage = "message"
	keyType    = "type"
)
