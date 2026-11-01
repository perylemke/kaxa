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
		img.Status, img.Error = statusFailed, "disk full"
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

// fakeResult is what a fake command does.
type fakeResult struct {
	stdout, stderr string
	code           int
	// err is a failure of the transport, such as a lost sandbox.
	err error
	// running keeps a spawned command alive.
	running bool
}

// fakeToolbox is an in-memory toolboxClient. The handler decides what each
// command prints and how it ends.
type fakeToolbox struct {
	mu      sync.Mutex
	handler func(spec runSpec, stdin string) fakeResult
	reqs    []runSpec
	stdins  []string
	uploads map[string]string
	// ttyOutput is what a terminal command prints.
	ttyOutput []string
	resizes   [][2]uint16
	startErr  error
	// holdForResize keeps a terminal command alive until a resize arrives.
	holdForResize bool
}

func newFakeToolbox(h func(runSpec, string) fakeResult) *fakeToolbox {
	return &fakeToolbox{handler: h, uploads: map[string]string{}}
}

func (f *fakeToolbox) record(spec runSpec, stdin string) fakeResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, spec)
	f.stdins = append(f.stdins, stdin)
	return f.handler(spec, stdin)
}

func (f *fakeToolbox) Run(_ context.Context, _ string, spec runSpec) (int, error) {
	if f.startErr != nil {
		return 0, f.startErr
	}
	stdin := ""
	if spec.stdin != nil && !spec.tty {
		data, _ := io.ReadAll(spec.stdin)
		stdin = string(data)
	}
	res := f.record(spec, stdin)
	if res.err != nil {
		return 0, res.err
	}
	if spec.tty {
		f.runTerminal(spec)
		return res.code, nil
	}
	if spec.stdout != nil {
		_, _ = io.WriteString(spec.stdout, res.stdout)
	}
	if spec.stderr != nil {
		_, _ = io.WriteString(spec.stderr, res.stderr)
	}
	return res.code, nil
}

func (f *fakeToolbox) runTerminal(spec runSpec) {
	for _, chunk := range f.ttyOutput {
		_, _ = io.WriteString(spec.stdout, chunk)
	}
	if spec.resize == nil {
		return
	}
	if !f.holdForResize {
		return
	}
	select {
	case size := <-spec.resize:
		f.mu.Lock()
		f.resizes = append(f.resizes, [2]uint16{size.Rows, size.Cols})
		f.mu.Unlock()
	case <-time.After(2 * time.Second):
	}
}

func (f *fakeToolbox) Spawn(_ context.Context, _ string, spec runSpec) (process, error) {
	if f.startErr != nil {
		return nil, f.startErr
	}
	res := f.record(spec, "")
	if res.err != nil {
		return nil, res.err
	}
	return &fakeProcess{res: res}, nil
}

func (f *fakeToolbox) Upload(_ context.Context, _, path string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.uploads[path] = string(data)
	return nil
}

// argvs returns the commands that were run.
func (f *fakeToolbox) argvs() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out [][]string
	for _, r := range f.reqs {
		out = append(out, r.argv)
	}
	return out
}

type fakeProcess struct{ res fakeResult }

func (p *fakeProcess) Wait(context.Context, time.Duration) (int, bool, error) {
	return p.res.code, !p.res.running, nil
}

func (p *fakeProcess) Output(context.Context) string { return p.res.stdout + p.res.stderr }

func okHandler(runSpec, string) fakeResult { return fakeResult{} }

// newTestProvider returns a provider over the fakes with fast polling.
func newTestProvider(t interface{ TempDir() string }, tb *fakeToolbox) (*Provider, *fakeAPI) {
	api := newFakeAPI()
	p := NewWithClients(api, tb, "test-key")
	p.SetPolling(time.Second, time.Millisecond)
	p.SetReadiness(time.Second, time.Millisecond)
	p.SetSnapshotIndexPath(t.TempDir() + "/snapshots.json")
	return p, api
}
