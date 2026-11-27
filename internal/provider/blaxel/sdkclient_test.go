package blaxel

import (
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

	sdk "github.com/blaxel-ai/sdk-go"
	"github.com/blaxel-ai/sdk-go/option"
)

// sdkServer is a fake Blaxel control and data plane for the real sdkClient.
type sdkServer struct {
	srv   *httptest.Server
	mu    sync.Mutex
	calls []string
	// routes maps "METHOD /path" to a status and JSON body.
	routes map[string]sdkReply
}

func (f *sdkServer) set(key string, rep sdkReply) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[key] = rep
}

func (f *sdkServer) del(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.routes, key)
}

type sdkReply struct {
	status int
	body   string
}

func newSDKServer(t *testing.T, routes map[string]sdkReply) (*sdkServer, *sdkClient) {
	t.Helper()
	fs := &sdkServer{routes: routes}
	fs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		fs.mu.Lock()
		fs.calls = append(fs.calls, key)
		rep, ok := fs.routes[key]
		fs.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"no route"}`))
			return
		}
		if rep.status != 0 {
			w.WriteHeader(rep.status)
		}
		_, _ = w.Write([]byte(rep.body))
	}))
	t.Cleanup(fs.srv.Close)
	opts := []option.RequestOption{
		option.WithBaseURL(fs.srv.URL), option.WithAPIKey(sdkTestKey), option.WithWorkspace("ws"),
		option.WithMaxRetries(0),
	}
	svc := sdk.NewSandboxService(opts...)
	snaps := sdk.NewSnapshotService(opts...)
	ss := sdk.NewSandboxSnapshotService(opts...)
	return fs, &sdkClient{
		apiKey: sdkTestKey, workspace: "ws", opts: opts, sandboxes: &svc, snapshots: &snaps,
		sandboxSnapshots: &ss, http: &http.Client{CheckRedirect: refuseRedirect},
	}
}

func sandboxJSON(t *testing.T, url string, managed bool) string {
	t.Helper()
	labels := map[string]string{}
	if managed {
		labels[labelManaged] = labelTrue
		labels[labelName] = boxName
	}
	raw, err := json.Marshal(map[string]any{
		"metadata": map[string]any{"name": boxName, "url": url, "labels": labels},
		"spec":     map[string]any{"region": defaultRegion, "runtime": map[string]any{"image": "img"}},
		"status":   phaseDeployed,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

const (
	sdkTestKey    = "test-key"
	getBox        = "GET /sandboxes/box"
	phaseDeployed = "DEPLOYED"
)

const procDone = `{"name":"p1","pid":"1","status":"completed","exitCode":3,"stdout":"out\n","stderr":"err\n"}`

const previewJSON = `{"metadata":{"name":"kaxa-api"},"spec":{"url":"https://p.preview.bl.run","public":true,"port":8000}}`

func TestSDKClientControlPlaneHub(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, c := newSDKServer(t, map[string]sdkReply{
		"GET /sandbox/hub": {body: `[{"image":"i","displayName":"I","memory":2048,"hidden":true,"comingSoon":false}]`},
	})
	got, err := c.ListHub(ctx)
	if err != nil || len(got) != 1 || got[0].Image != "i" || got[0].Memory != 2048 || !got[0].Hidden {
		t.Fatalf("hub = %+v, %v", got, err)
	}
	_, bad := newSDKServer(t, nil)
	if _, err := bad.ListHub(ctx); err == nil {
		t.Fatal("missing hub route did not fail")
	}
}

func TestSDKClientControlPlaneCreate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fs, c := newSDKServer(t, nil)
	fs.routes = map[string]sdkReply{"POST /sandboxes": {body: sandboxJSON(t, fs.srv.URL, true)}}
	rec, err := c.Create(ctx, CreateParams{Name: boxName, Image: "img", Region: defaultRegion, Memory: 2048, DiskPercent: 50, APIPort: 8000})
	if err != nil || rec.Name != boxName {
		t.Fatalf("rec = %+v, %v", rec, err)
	}
	fs.set("POST /sandboxes", sdkReply{body: sandboxJSON(t, fs.srv.URL, false)})
	if _, err := c.Create(ctx, CreateParams{Name: boxName, Region: defaultRegion}); err == nil {
		t.Fatal("unmanaged create accepted")
	}
	fs.set("POST /sandboxes", sdkReply{status: http.StatusConflict, body: `{"error":"x"}`})
	if _, err := c.Create(ctx, CreateParams{Name: boxName, Region: defaultRegion}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("err = %v", err)
	}
}

func TestSDKClientControlPlaneList(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fs, c := newSDKServer(t, nil)
	fs.routes = map[string]sdkReply{"GET /sandboxes": {body: `{"data":[` + sandboxJSON(t, "", true) + "," + sandboxJSON(t, "", false) + `],"meta":{"hasMore":false}}`}}
	got, err := c.List(ctx)
	if err != nil || len(got) != 1 {
		t.Fatalf("list = %+v, %v", got, err)
	}
	fs.set("GET /sandboxes", sdkReply{status: http.StatusNotFound})
	if _, err := c.List(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestSDKClientControlPlaneFindSandboxDelete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fs, c := newSDKServer(t, nil)
	fs.routes = map[string]sdkReply{
		getBox:                  {body: sandboxJSON(t, fs.srv.URL, true)},
		"GET /sandboxes/other":  {body: sandboxJSON(t, fs.srv.URL, false)},
		"DELETE /sandboxes/box": {body: sandboxJSON(t, fs.srv.URL, true)},
	}
	if rec, err := c.Find(ctx, boxName); err != nil || rec.Name != boxName {
		t.Fatalf("find = %+v, %v", rec, err)
	}
	if _, err := c.Find(ctx, "other"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unmanaged find = %v", err)
	}
	if _, err := c.Find(ctx, "gone"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing find = %v", err)
	}
	if sb, err := c.Sandbox(ctx, boxName); err != nil || sb == nil {
		t.Fatalf("sandbox = %v, %v", sb, err)
	}
	if _, err := c.Sandbox(ctx, "gone"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing sandbox = %v", err)
	}
	if err := c.Delete(ctx, boxName); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(ctx, "gone"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete = %v", err)
	}
}

func TestSDKClientControlPlanePreviews(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fs, c := newSDKServer(t, nil)
	fs.routes = map[string]sdkReply{
		getBox:                                         {body: sandboxJSON(t, fs.srv.URL, true)},
		"GET /sandboxes/box/previews/kaxa-api":         {body: previewJSON},
		"POST /sandboxes/box/previews":                 {body: previewJSON},
		"POST /sandboxes/box/previews/kaxa-api/tokens": {body: `{"spec":{"token":"tok"}}`},
	}
	if u, public, err := c.PreviewURL(ctx, boxName); err != nil || !public || u == "" {
		t.Fatalf("preview = %q %v %v", u, public, err)
	}
	if _, _, err := c.PreviewURL(ctx, "gone"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if u, tok, err := c.CreatePreview(ctx, boxName, 8000, true); err != nil || tok != "" || u == "" {
		t.Fatalf("public = %q %q %v", u, tok, err)
	}
	if u, tok, err := c.CreatePreview(ctx, boxName, 8000, false); err != nil || tok != "tok" || u == "" {
		t.Fatalf("private = %q %q %v", u, tok, err)
	}
	fs.set("POST /sandboxes/box/previews/kaxa-api/tokens", sdkReply{body: `{"spec":{"token":""}}`})
	if _, _, err := c.CreatePreview(ctx, boxName, 8000, false); err == nil {
		t.Fatal("empty token accepted")
	}
	fs.del("POST /sandboxes/box/previews/kaxa-api/tokens")
	if _, _, err := c.CreatePreview(ctx, boxName, 8000, false); err == nil {
		t.Fatal("token failure ignored")
	}
}

func TestSDKClientControlPlanePreviewFailures(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fs, c := newSDKServer(t, nil)
	fs.routes = map[string]sdkReply{getBox: {body: sandboxJSON(t, fs.srv.URL, true)}}
	fs.set("POST /sandboxes/box/previews", sdkReply{body: `{"metadata":{"name":"kaxa-api"},"spec":{"url":""}}`})
	if _, _, err := c.CreatePreview(ctx, boxName, 8000, true); err == nil {
		t.Fatal("empty preview url accepted")
	}
	fs.del("POST /sandboxes/box/previews")
	if _, _, err := c.CreatePreview(ctx, boxName, 8000, true); err == nil {
		t.Fatal("create failure ignored")
	}
	if _, _, err := c.CreatePreview(ctx, "gone", 8000, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	fs.set("GET /sandboxes/box/previews/kaxa-api", sdkReply{body: `{"spec":{}}`})
	if _, _, err := c.PreviewURL(ctx, boxName); err == nil {
		t.Fatal("empty preview url accepted")
	}
	fs.del("GET /sandboxes/box/previews/kaxa-api")
	if _, _, err := c.PreviewURL(ctx, boxName); err == nil {
		t.Fatal("missing preview accepted")
	}
}

func TestSDKClientDataPlaneRun(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fs, c := newSDKServer(t, nil)
	fs.routes = map[string]sdkReply{
		getBox:            {body: sandboxJSON(t, fs.srv.URL, true)},
		"POST /process":   {body: procDone},
		"GET /process/p1": {body: procDone},
	}
	var out, errw strings.Builder
	code, err := c.Run(ctx, boxName, runSpec{argv: []string{"x"}, dir: "/w", env: []string{"A=b"}, stdout: &out, stderr: &errw})
	if err != nil || code != 3 || !strings.Contains(out.String(), "out") || !strings.Contains(errw.String(), "err") {
		t.Fatalf("run = %d %v %q %q", code, err, out.String(), errw.String())
	}
}

func TestSDKClientDataPlaneRunFallsBackToLogs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fs, c := newSDKServer(t, nil)
	empty := `{"name":"p1","pid":"1","status":"completed","exitCode":0,"stdout":"","stderr":""}`
	fs.routes = map[string]sdkReply{
		getBox:                 {body: sandboxJSON(t, fs.srv.URL, true)},
		"POST /process":        {body: empty},
		"GET /process/p1":      {body: empty},
		"GET /process/p1/logs": {body: `{"stdout":"late\n","stderr":""}`},
	}
	var out strings.Builder
	if code, err := c.Run(ctx, boxName, runSpec{argv: []string{"x"}, stdout: &out}); err != nil || code != 0 {
		t.Fatalf("run = %d %v", code, err)
	}
	fs.del("GET /process/p1/logs")
	if _, err := c.Run(ctx, boxName, runSpec{argv: []string{"x"}}); err == nil {
		t.Fatal("logs failure ignored")
	}
}

func TestSDKClientDataPlaneRunErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fs, c := newSDKServer(t, nil)
	fs.routes = map[string]sdkReply{getBox: {body: sandboxJSON(t, fs.srv.URL, true)}}
	if _, err := c.Run(ctx, boxName, runSpec{argv: []string{"x"}}); err == nil {
		t.Fatal("process create failure ignored")
	}
	if _, err := c.Run(ctx, "gone", runSpec{argv: []string{"x"}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	fs.set("POST /process", sdkReply{body: procDone})
	if _, err := c.Run(ctx, boxName, runSpec{argv: []string{"x"}}); err == nil {
		t.Fatal("wait failure ignored")
	}
	// Stdin goes to a non-Blaxel host, which is refused before any request.
	fs.set("GET /process/p1", sdkReply{body: procDone})
	if _, err := c.Run(ctx, boxName, runSpec{argv: []string{"x"}, stdin: strings.NewReader("in")}); err == nil {
		t.Fatal("stdin to a non-Blaxel host accepted")
	}
}

func TestSDKClientDataPlaneStart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fs, c := newSDKServer(t, nil)
	running := `{"name":"p1","pid":"1","status":"running"}`
	fs.routes = map[string]sdkReply{
		getBox:          {body: sandboxJSON(t, fs.srv.URL, true)},
		"POST /process": {body: running},
	}
	if err := c.Start(ctx, boxName, runSpec{argv: []string{"x"}, port: 8000, stdin: strings.NewReader("")}); err != nil {
		t.Fatal(err)
	}
	fs.set("POST /process", sdkReply{body: `{"name":"p1","status":"failed","exitCode":2,"stderr":"boom"}`})
	if err := c.Start(ctx, boxName, runSpec{argv: []string{"x"}, port: 8000}); err == nil {
		t.Fatal("exited service accepted")
	}
	if err := c.Start(ctx, "gone", runSpec{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestSDKClientDataPlaneFiles(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fs, c := newSDKServer(t, nil)
	fs.routes = map[string]sdkReply{
		getBox:                  {body: sandboxJSON(t, fs.srv.URL, true)},
		"PUT /filesystem//a":    {body: `{"message":"ok","path":"/a"}`},
		"DELETE /filesystem//a": {body: `{"message":"ok","path":"/a"}`},
	}
	if err := c.Upload(ctx, boxName, "/a", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := c.Remove(ctx, boxName, "/a"); err != nil {
		t.Fatal(err)
	}
	if err := c.Upload(ctx, "gone", "/a", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if err := c.Remove(ctx, "gone", "/a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestSDKClientDataPlaneNoUrl(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fs, c := newSDKServer(t, nil)
	fs.routes = map[string]sdkReply{getBox: {body: sandboxJSON(t, "", true)}}
	if _, err := c.instance(ctx, boxName); err == nil {
		t.Fatal("sandbox without URL accepted")
	}
	if err := c.Upload(ctx, boxName, "/a", nil); err == nil {
		t.Fatal("upload without URL accepted")
	}
	if err := c.Remove(ctx, boxName, "/a"); err == nil {
		t.Fatal("remove without URL accepted")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestStdinRequest(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var (
		mu      sync.Mutex
		methods []string
		bearer  string
	)
	status := http.StatusOK
	c := newSDKClient(sdkTestKey, "ws")
	c.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		methods = append(methods, r.Method)
		bearer = r.Header.Get("Authorization")
		code := status
		mu.Unlock()
		return &http.Response{
			StatusCode: code, Body: io.NopCloser(strings.NewReader("denied test-key")),
			Header: http.Header{}, Request: r,
		}, nil
	})}
	const base = "https://x.us-pdx-1.bl.run"
	if err := c.stdinRequest(ctx, http.MethodPost, "https://x.example.com", "p1", nil); err == nil {
		t.Fatal("non-Blaxel host accepted")
	}
	if err := c.stdinRequest(ctx, http.MethodPost, base, "../x", nil); err == nil {
		t.Fatal("bad id accepted")
	}
	if err := c.stdinRequest(ctx, "bad method", base, "p1", nil); err == nil {
		t.Fatal("bad method accepted")
	}
	inst := &sdk.SandboxInstance{Sandbox: &sdk.Sandbox{}}
	inst.Metadata.URL = base
	if err := c.writeStdin(ctx, inst, "p1", strings.NewReader("in")); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	gotMethods, gotBearer := strings.Join(methods, ","), bearer
	status = http.StatusBadGateway
	mu.Unlock()
	if gotMethods != "POST,DELETE" || gotBearer != "Bearer test-key" {
		t.Fatalf("methods = %q, bearer set = %v", gotMethods, gotBearer != "")
	}
	err := c.writeStdin(ctx, inst, "p1", strings.NewReader("in"))
	if err == nil || strings.Contains(err.Error(), sdkTestKey) {
		t.Fatalf("err = %v", err)
	}
	c.http = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial test-key")
	})}
	if err := c.stdinRequest(ctx, http.MethodDelete, base, "p1", nil); err == nil || strings.Contains(err.Error(), sdkTestKey) {
		t.Fatalf("err = %v", err)
	}
}

func TestSDKClientRedact(t *testing.T) {
	t.Parallel()
	c := newSDKClient("sec ret", "ws")
	if got := c.redact(errors.New("a sec ret b sec+ret")); strings.Contains(got.Error(), "sec") {
		t.Fatalf("got %v", got)
	}
	if err := errors.New("plain"); !errors.Is(c.redact(err), err) {
		t.Fatal("clean error changed")
	}
	if c.redact(nil) != nil {
		t.Fatal("nil changed")
	}
	if newSDKClient("", "ws").redact(errors.New("x")) == nil {
		t.Fatal("empty key dropped error")
	}
}

func TestClientHelpers(t *testing.T) {
	t.Parallel()
	if !errors.Is(refuseRedirect(nil, nil), http.ErrUseLastResponse) {
		t.Fatal("redirect followed")
	}
	if got := portParams(0); got != nil {
		t.Fatalf("ports = %v", got)
	}
	if got := portParams(8000); len(got) != 1 || got[0].Target != 8000 {
		t.Fatalf("ports = %v", got)
	}
	if got := envMap(nil); got != nil {
		t.Fatalf("env = %v", got)
	}
	if got := envMap([]string{"junk"}); got != nil {
		t.Fatalf("env = %v", got)
	}
	if got := envMap([]string{"A=b", "C=d=e"}); got["A"] != "b" || got["C"] != "d=e" {
		t.Fatalf("env = %v", got)
	}
	if got := trimOutput("short"); got != "short" {
		t.Fatalf("trim = %q", got)
	}
	if got := trimOutput(strings.Repeat("x", failureOutput+10)); len(got) != failureOutput {
		t.Fatalf("trim len = %d", len(got))
	}
}

func TestClientHelpersParsing(t *testing.T) {
	t.Parallel()
	if parseTime("junk") != (time.Time{}) || parseTime("2026-01-02T03:04:05.5Z").IsZero() {
		t.Fatal("parseTime")
	}
	if name, err := procName(); err != nil || !strings.HasPrefix(name, "kaxa-") {
		t.Fatalf("name = %q, %v", name, err)
	}
	if processID(nil, "fb") != "fb" || processID(&sdk.ProcessResponse{Pid: "9"}, "fb") != "9" ||
		processID(&sdk.ProcessResponse{Name: "n", Pid: "9"}, "fb") != "n" {
		t.Fatal("processID")
	}
	if serviceExited(sdk.ProcessResponseStatusRunning) || !serviceExited(sdk.ProcessResponseStatusFailed) ||
		!serviceExited("weird") {
		t.Fatal("serviceExited")
	}
}

func TestClientHelpersOutput(t *testing.T) {
	t.Parallel()
	if pendingOutput("", "abc") != "abc" || pendingOutput("ab", "abc") != "c" || pendingOutput("x", "abc") != "" {
		t.Fatal("pendingOutput")
	}
	if streamFrame("") != "" || streamFrame("a") != "a\n" || streamFrame("a\n") != "a\n" {
		t.Fatal("streamFrame")
	}
}

func TestClientHelpersRecords(t *testing.T) {
	t.Parallel()
	if _, err := previewURL(nil); err == nil {
		t.Fatal("nil preview accepted")
	}
	if got := managedRecords([]*sdk.SandboxInstance{nil, {}}); len(got) != 0 {
		t.Fatalf("records = %v", got)
	}
	closeStream(nil)
	closeStream(&sdk.StreamControl{})
	if !nameFree(Record{Phase: "TERMINATED"}) || nameFree(Record{Phase: phaseDeployed}) {
		t.Fatal("nameFree")
	}
	body := processBody(runSpec{argv: []string{"a b"}, dir: "/w", stdin: strings.NewReader(""), port: 80, env: []string{"A=b"}}, "n", true)
	if !body.Stdin.Valid() || len(body.WaitForPorts) != 1 || !body.WorkingDir.Valid() {
		t.Fatalf("body = %+v", body)
	}
	if body := processBody(runSpec{argv: []string{"a"}, port: 80}, "n", false); len(body.WaitForPorts) != 0 {
		t.Fatal("port waited without waitPort")
	}
	var o processOutput
	var w strings.Builder
	o.write(&w, &o.stdout, "")
	o.write(&w, &o.stdout, "x")
	if !o.wrote() || w.String() != "x\n" {
		t.Fatalf("out = %q", w.String())
	}
	writeOut(nil, "x")
	if got := apiDetail(&sdk.Error{}); got != "" {
		t.Fatalf("detail = %q", got)
	}
	withBody := &sdk.Error{}
	_ = withBody.UnmarshalJSON([]byte(`{"error":"boom"}`))
	if got := apiDetail(withBody); got != "boom" {
		t.Fatalf("detail = %q", got)
	}
}
