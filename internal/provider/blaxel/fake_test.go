package blaxel

import (
	"context"
	"io"
	"slices"
	"sync"
	"testing"
	"time"

	sdk "github.com/blaxel-ai/sdk-go"
)

const (
	boxName   = "box"
	apiName   = "api"
	otherName = "other-image"
)

type fakeResult struct {
	code           int
	stdout, stderr string
	err            error
}

type fakeAPI struct {
	mu           sync.Mutex
	sandboxes    map[string]*Record
	lastCreate   CreateParams
	previews     map[string]string
	createStatus string
	createErr    error
	commands     [][]string
	dirs         []string
	envs         [][]string
	stdins       []string
	uploads      map[string]string
	handler      func(runSpec) fakeResult
	blindFind    bool
	// lingerAfterDelete is how many Finds still see the sandbox after Delete.
	// Negative means the name stays in DELETING. Zero removes it immediately.
	lingerAfterDelete int
	// terminateAfterDelete leaves the sandbox on Get with a terminated phase.
	terminateAfterDelete bool
	readsLeft            int
	terms                []termCall
	termOut              string
	termErr              error
	hub                  []hubTemplate
}

type termCall struct {
	dir        string
	rows, cols uint16
	stdin      string
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{
		sandboxes: map[string]*Record{}, previews: map[string]string{},
		uploads: map[string]string{}, createStatus: statusRunning,
	}
}

func newTestProvider(t *testing.T, api *fakeAPI) *Provider {
	t.Helper()
	p := NewWithClients(api, api, "test-key", "kaxa")
	p.SetPolling(time.Second, time.Millisecond)
	p.SetReadiness(time.Second, time.Millisecond)
	return p
}

func (f *fakeAPI) ListHub(context.Context) ([]hubTemplate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.hub), nil
}

func (f *fakeAPI) Create(_ context.Context, p CreateParams) (Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return Record{}, f.createErr
	}
	if rec, ok := f.sandboxes[p.Name]; ok && !nameFree(*rec) {
		return Record{}, ErrAlreadyExists
	}
	f.lastCreate = p
	rec := &Record{
		Name: p.Name, Status: f.createStatus, Labels: slices.Clone(p.Labels),
		Image: p.Image, Region: p.Region, APIPort: p.APIPort, CreatedAt: time.Unix(1_700_000_000, 0),
	}
	f.sandboxes[p.Name] = rec
	return *rec, nil
}

func (f *fakeAPI) List(context.Context) ([]Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Record, 0, len(f.sandboxes))
	for _, rec := range f.sandboxes {
		out = append(out, *rec)
	}
	return out, nil
}

func (f *fakeAPI) Find(_ context.Context, name string) (Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec, ok := f.sandboxes[name]
	if !ok || f.blindFind {
		return Record{}, ErrNotFound
	}
	if f.readsLeft > 0 {
		f.readsLeft--
		if f.readsLeft == 0 {
			delete(f.sandboxes, name)
			return Record{}, ErrNotFound
		}
	}
	if rec.Status == statusStarting {
		rec.Status = statusRunning
	}
	return *rec, nil
}

func (f *fakeAPI) Sandbox(_ context.Context, name string) (*sdk.Sandbox, error) {
	rec, err := f.Find(context.Background(), name)
	if err != nil {
		return nil, err
	}
	return &sdk.Sandbox{
		Metadata: sdk.Metadata{Name: rec.Name, CreatedAt: rec.CreatedAt.Format(time.RFC3339), Workspace: "kaxa"},
		Spec: sdk.SandboxSpec{
			Region:  rec.Region,
			Runtime: sdk.SandboxRuntime{Image: rec.Image, Memory: 4096},
		},
	}, nil
}

func (f *fakeAPI) Delete(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.sandboxes[name]; !ok {
		return ErrNotFound
	}
	if f.terminateAfterDelete {
		f.sandboxes[name].Status = statusStopped
		f.sandboxes[name].Phase = phaseTerminated
		return nil
	}
	if f.lingerAfterDelete != 0 {
		f.readsLeft = f.lingerAfterDelete
		f.sandboxes[name].Status = statusStopped
		f.sandboxes[name].Phase = phaseDeleting
		return nil
	}
	delete(f.sandboxes, name)
	return nil
}

func (f *fakeAPI) PreviewURL(_ context.Context, name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.sandboxes[name]; !ok {
		return "", ErrNotFound
	}
	url, ok := f.previews[name]
	if !ok {
		return "", ErrNotFound
	}
	return url, nil
}

func (f *fakeAPI) CreatePreview(_ context.Context, name string, port int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rec, ok := f.sandboxes[name]
	if !ok {
		return "", ErrNotFound
	}
	url := "https://" + name + ".preview.example"
	f.previews[name] = url
	rec.APIPort = port
	return url, nil
}

func (f *fakeAPI) Run(_ context.Context, name string, spec runSpec) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.sandboxes[name]; !ok {
		return 0, ErrNotFound
	}
	f.commands = append(f.commands, slices.Clone(spec.argv))
	f.dirs = append(f.dirs, spec.dir)
	f.envs = append(f.envs, slices.Clone(spec.env))
	var stdin string
	if spec.stdin != nil {
		b, _ := io.ReadAll(spec.stdin)
		stdin = string(b)
	}
	f.stdins = append(f.stdins, stdin)
	res := fakeResult{}
	if f.handler != nil {
		res = f.handler(spec)
	}
	if spec.stdout != nil && res.stdout != "" {
		_, _ = io.WriteString(spec.stdout, res.stdout)
	}
	if spec.stderr != nil && res.stderr != "" {
		_, _ = io.WriteString(spec.stderr, res.stderr)
	}
	return res.code, res.err
}

func (f *fakeAPI) Start(ctx context.Context, name string, spec runSpec) error {
	code, err := f.Run(ctx, name, spec)
	if err != nil {
		return err
	}
	if code != 0 {
		return apiServiceFailed(code, "")
	}
	return nil
}

func (f *fakeAPI) Upload(_ context.Context, name, path string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.sandboxes[name]; !ok {
		return ErrNotFound
	}
	f.uploads[path] = string(data)
	return nil
}

func (f *fakeAPI) Remove(context.Context, string, string) error { return nil }

func (f *fakeAPI) Terminal(_ context.Context, name string, spec termSpec) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.sandboxes[name]; !ok {
		return ErrNotFound
	}
	var stdin string
	if spec.stdin != nil {
		b, _ := io.ReadAll(spec.stdin)
		stdin = string(b)
	}
	f.terms = append(f.terms, termCall{dir: spec.dir, rows: spec.rows, cols: spec.cols, stdin: stdin})
	if spec.stdout != nil && f.termOut != "" {
		_, _ = io.WriteString(spec.stdout, f.termOut)
	}
	return f.termErr
}

func (f *fakeAPI) add(name, status string, labels ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sandboxes[name] = &Record{Name: name, Status: status, Labels: labels, Region: defaultRegion, Image: defaultImage}
}

func (f *fakeAPI) argvs() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.commands)
}

func commandIs(argv []string, want ...string) bool {
	return slices.Equal(argv, want)
}
