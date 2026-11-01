package daytona

import (
	"context"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	boxName    = "box"
	otherName  = "other"
	newName    = "new"
	bashCmd    = "bash"
	readyState = "ready"
	sandboxID  = "sb-1"
	labelsKey  = "labels"
)

// expect fails the test when got differs from exp.
func expect(t *testing.T, what string, got, exp any) {
	t.Helper()
	if !reflect.DeepEqual(got, exp) {
		t.Errorf("%s = %v, want %v", what, got, exp)
	}
}

// fakeAPI is an in-memory apiClient.
type fakeAPI struct {
	mu        sync.Mutex
	seq       int
	sandboxes map[string]*Record
	images    map[string]*Image
	ports     map[string]PortEndpoint
	// createStatus is the status a new sandbox starts with. After that each Get
	// moves queued to booting to running.
	createStatus string
	calls        []string
	// failImage makes CaptureImage produce a failed image.
	failImage bool
	// imageStatus is the status a captured image starts with; it turns ready on Get.
	imageStatus string
	deleteErr   error
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{
		sandboxes: map[string]*Record{}, images: map[string]*Image{}, ports: map[string]PortEndpoint{},
		createStatus: statusRunning, imageStatus: "pending",
	}
}

func (f *fakeAPI) record(call string) { f.calls = append(f.calls, call) }

func (f *fakeAPI) Create(_ context.Context, p CreateParams) (Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("create " + p.Name)
	f.seq++
	rec := &Record{
		ID: "sb-" + strconv.Itoa(f.seq), Name: p.Name, Labels: slices.Clone(p.Labels), Status: f.createStatus,
		CreatedAt: time.Unix(int64(1000+f.seq), 0), Template: p.Template, APIPort: p.APIPort,
		ToolboxURL: "https://toolbox.example.test",
	}
	f.sandboxes[rec.ID] = rec
	return *rec, nil
}

func (f *fakeAPI) List(context.Context) ([]Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Record
	for _, k := range slices.Sorted(maps.Keys(f.sandboxes)) {
		out = append(out, *f.sandboxes[k])
	}
	return out, nil
}

func (f *fakeAPI) Find(ctx context.Context, name string) (Record, error) {
	recs, _ := f.List(ctx)
	var best Record
	found := false
	for _, r := range recs {
		if r.Name == name && (!found || r.CreatedAt.After(best.CreatedAt)) {
			best, found = r, true
		}
	}
	if !found {
		return Record{}, ErrNotFound
	}
	return best, nil
}

func (f *fakeAPI) Get(_ context.Context, id string) (Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec, ok := f.sandboxes[id]
	if !ok {
		return Record{}, ErrNotFound
	}
	switch rec.Status {
	case "queued":
		rec.Status = "booting"
	case "booting":
		rec.Status = statusRunning
	}
	return *rec, nil
}

func (f *fakeAPI) Delete(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("delete " + id)
	if _, ok := f.sandboxes[id]; !ok {
		return ErrNotFound
	}
	delete(f.sandboxes, id)
	return nil
}

func (f *fakeAPI) setStatus(id, status, call string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record(call + " " + id)
	rec, ok := f.sandboxes[id]
	if !ok {
		return ErrNotFound
	}
	rec.Status = status
	return nil
}

func (f *fakeAPI) Start(_ context.Context, id string) error {
	return f.setStatus(id, "booting", "start")
}

func (f *fakeAPI) Resume(_ context.Context, id string) error {
	return f.setStatus(id, "booting", "resume")
}

func (f *fakeAPI) Stop(_ context.Context, id string) error {
	return f.setStatus(id, statusStopped, "stop")
}

func (f *fakeAPI) SetPort(_ context.Context, id string, port int, public bool) (PortEndpoint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("setport " + id + " " + strconv.Itoa(port))
	ep := PortEndpoint{Port: port, Public: public, URL: fmt.Sprintf("https://%d-%s.proxy.example.test", port, id)}
	f.ports[id+":"+strconv.Itoa(port)] = ep
	return ep, nil
}

func (f *fakeAPI) GetPort(_ context.Context, id string, port int) (PortEndpoint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ep, ok := f.ports[id+":"+strconv.Itoa(port)]
	if !ok {
		return PortEndpoint{}, ErrNotFound
	}
	return ep, nil
}

func (f *fakeAPI) CaptureImage(_ context.Context, id, name string) (Image, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("capture " + id)
	if _, ok := f.sandboxes[id]; !ok {
		return Image{}, ErrNotFound
	}
	f.seq++
	img := &Image{ID: "img_" + strconv.Itoa(f.seq), Name: name, Status: f.imageStatus, CreatedAt: time.Unix(int64(5000+f.seq), 0)}
	if f.failImage {
		img.Status, img.Error = "failed", "disk full"
	}
	f.images[name] = img
	return *img, nil
}

func (f *fakeAPI) ListImages(_ context.Context, prefix string) ([]Image, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Image
	for _, k := range slices.Sorted(maps.Keys(f.images)) {
		if strings.HasPrefix(k, prefix) {
			out = append(out, *f.images[k])
		}
	}
	return out, nil
}

func (f *fakeAPI) GetImage(_ context.Context, name string) (Image, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	img, ok := f.images[name]
	if !ok {
		return Image{}, ErrImageNotFound
	}
	if img.Status == "pending" {
		img.Status = readyState
	}
	return *img, nil
}

func (f *fakeAPI) DeleteImage(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("deleteimage " + id)
	if f.deleteErr != nil {
		return f.deleteErr
	}
	for k, img := range f.images {
		if img.ID == id {
			delete(f.images, k)
			return nil
		}
	}
	return ErrImageNotFound
}

func (f *fakeAPI) has(call string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.calls, call)
}

// add puts a managed sandbox in the fake.
func (f *fakeAPI) add(name, status string, labels ...string) string {
	rec, _ := f.Create(context.Background(), CreateParams{Name: name, Labels: labels})
	f.mu.Lock()
	f.sandboxes[rec.ID].Status = status
	f.mu.Unlock()
	return rec.ID
}

// fakeResult is what a fake process does.
type fakeResult struct {
	stdout, stderr string
	code           int
	reason         string
	// running keeps the process alive.
	running bool
}

type fakeProc struct {
	req    ProcessRequest
	frames []LogFrame
	res    fakeResult
	done   bool
	stdin  strings.Builder
}

// fakeToolbox is an in-memory toolboxClient. The handler decides what each
// started process prints and how it ends.
type fakeToolbox struct {
	mu      sync.Mutex
	handler func(req ProcessRequest, stdin string) fakeResult
	procs   map[string]*fakeProc
	reqs    []ProcessRequest
	uploads map[string]string
	// attachOutput is what a pty process prints.
	attachOutput []string
	attached     *fakeAttachment
	resizes      [][2]uint16
	startErr     error
	// holdForResize keeps a pty stream open until a resize arrives.
	holdForResize bool
}

func newFakeToolbox(h func(ProcessRequest, string) fakeResult) *fakeToolbox {
	return &fakeToolbox{handler: h, procs: map[string]*fakeProc{}, uploads: map[string]string{}}
}

func (f *fakeToolbox) finish(p *fakeProc) {
	p.res = f.handler(p.req, p.stdin.String())
	if p.res.stdout != "" {
		p.frames = append(p.frames, LogFrame{Channel: "stdout", Data: p.res.stdout})
	}
	if p.res.stderr != "" {
		p.frames = append(p.frames, LogFrame{Channel: "stderr", Data: p.res.stderr})
	}
	for i := range p.frames {
		p.frames[i].Cursor = strconv.Itoa(i + 1)
	}
	p.done = !p.res.running
}

func (f *fakeToolbox) Start(_ context.Context, _ Conn, r ProcessRequest) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return "", f.startErr
	}
	f.reqs = append(f.reqs, r)
	id := "proc-" + strconv.Itoa(len(f.reqs))
	p := &fakeProc{req: r}
	f.procs[id] = p
	if !r.Stdin {
		f.finish(p)
	}
	return id, nil
}

func (f *fakeToolbox) Logs(_ context.Context, _ Conn, id, cursor string) ([]LogFrame, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.procs[id]
	from := 0
	if cursor != "" {
		from, _ = strconv.Atoi(cursor)
	}
	if from >= len(p.frames) {
		return nil, nil
	}
	return slices.Clone(p.frames[from:]), nil
}

func (f *fakeToolbox) Wait(_ context.Context, _ Conn, id string, _ time.Duration) (WaitResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.procs[id]
	if !p.done {
		return WaitResult{Reason: reasonTimedOut}, nil
	}
	reason := p.res.reason
	if reason == "" {
		reason = reasonExited
	}
	return WaitResult{Reason: reason, ExitCode: p.res.code, HasCode: reason == reasonExited}, nil
}

func (f *fakeToolbox) Stdin(_ context.Context, _ Conn, id string, data []byte, eof bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.procs[id]
	p.stdin.Write(data)
	if eof && !p.done {
		f.finish(p)
	}
	return nil
}

func (f *fakeToolbox) Resize(_ context.Context, _ Conn, _ string, rows, cols uint16) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resizes = append(f.resizes, [2]uint16{rows, cols})
	return nil
}

func (f *fakeToolbox) Attach(_ context.Context, _ Conn, id string) (Attachment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := &fakeAttachment{chunks: slices.Clone(f.attachOutput)}
	if f.holdForResize {
		a.hold = func() bool {
			f.mu.Lock()
			defer f.mu.Unlock()
			return len(f.resizes) == 0
		}
	}
	f.attached = a
	f.finish(f.procs[id])
	return a, nil
}

func (f *fakeToolbox) Upload(_ context.Context, _ Conn, path string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uploads[path] = string(data)
	return nil
}

// argvs returns the commands that were started, without the env(1) prefix.
func (f *fakeToolbox) argvs() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out [][]string
	for _, r := range f.reqs {
		out = append(out, plain(r.Argv))
	}
	return out
}

// plain drops a leading env(1) and its KEY=VALUE entries.
func plain(argv []string) []string {
	if len(argv) == 0 || argv[0] != "env" {
		return argv
	}
	i := 1
	for i < len(argv) && strings.Contains(argv[i], "=") && !strings.HasPrefix(argv[i], "/") {
		i++
	}
	return argv[i:]
}

type fakeAttachment struct {
	mu      sync.Mutex
	chunks  []string
	written []string
	closed  bool
	// hold keeps the stream open while it returns true.
	hold func() bool
}

func (a *fakeAttachment) Read() ([]byte, error) {
	if a.hold != nil {
		for range 400 {
			if !a.hold() {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.chunks) == 0 {
		return nil, io.EOF
	}
	c := a.chunks[0]
	a.chunks = a.chunks[1:]
	return []byte(c), nil
}

func (a *fakeAttachment) Write(data []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.written = append(a.written, string(data))
	return nil
}

func (a *fakeAttachment) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed = true
	return nil
}

func okHandler(ProcessRequest, string) fakeResult { return fakeResult{} }

// newTestProvider returns a provider over the fakes with fast polling.
func newTestProvider(t interface{ TempDir() string }, tb *fakeToolbox) (*Provider, *fakeAPI) {
	api := newFakeAPI()
	p := NewWithClients(api, tb, "test-key")
	p.SetPolling(time.Second, time.Millisecond)
	p.SetReadiness(time.Second, time.Millisecond)
	p.SetSnapshotIndexPath(t.TempDir() + "/snapshots.json")
	return p, api
}
