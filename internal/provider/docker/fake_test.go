package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"
)

const (
	testSandbox = "box"
	testToken   = "endpoint-secret-token" // #nosec G101 -- test fixture, not a real credential
	testPAT     = "dckr_pat_secret"       // #nosec G101 -- test fixture, not a real credential
	cmdEcho     = "echo"
	labelKaxa   = "kaxa"
	statusNew   = "starting"
	testImage   = "ubuntu:24.04"
	testRes     = "sandboxes/abc"
	cmdCat      = "cat"
	cmdEnv      = "env"
)

// fakeAPI is an in-memory management API. A sandbox changes status after one
// read, which makes the provider poll.
type fakeAPI struct {
	mu      sync.Mutex
	recs    map[string]*Record
	next    map[string]string
	n       int
	calls   []string
	failOn  map[string]error
	created []CreateParams
	renewed int
	// published lists the ports PublishPort was called with.
	published []int
	failure   string
	tokenOut  string
	// stuck keeps a new sandbox in starting.
	stuck bool
	// snapshots is created on first use; see fake_snapshot_test.go.
	snapshots *fakeSnapshots
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{recs: map[string]*Record{}, next: map[string]string{}, failOn: map[string]error{}, tokenOut: testToken}
}

func (f *fakeAPI) secrets() []string { return []string{testPAT} }

func (f *fakeAPI) record(verb string) error {
	f.calls = append(f.calls, verb)
	return f.failOn[verb]
}

func (f *fakeAPI) seen(verb string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == verb {
			n++
		}
	}
	return n
}

func (f *fakeAPI) List(context.Context) ([]Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("list"); err != nil {
		return nil, err
	}
	var out []Record
	for _, r := range f.recs {
		out = append(out, *r)
	}
	return out, nil
}

func (f *fakeAPI) Find(_ context.Context, name string) (Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("find"); err != nil {
		return Record{}, err
	}
	for _, r := range f.recs {
		if r.Name == name {
			return *r, nil
		}
	}
	return Record{}, ErrNotFound
}

func (f *fakeAPI) Get(_ context.Context, resource string) (Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("get"); err != nil {
		return Record{}, err
	}
	r, ok := f.recs[resource]
	if !ok {
		return Record{}, ErrNotFound
	}
	out := *r
	if next, ok := f.next[resource]; ok {
		delete(f.next, resource)
		if next == "gone" {
			delete(f.recs, resource)
		} else {
			r.Status = next
		}
	}
	return out, nil
}

func (f *fakeAPI) Create(_ context.Context, p CreateParams) (Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("create"); err != nil {
		return Record{}, err
	}
	f.n++
	res := fmt.Sprintf("sandboxes/id%d", f.n)
	r := &Record{
		Name: p.Name, Resource: res, Status: statusNew, Labels: p.Labels, Image: p.Image,
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Etag: `"e1"`,
		Endpoint: Endpoint{URI: "https://sbx.example.test/" + res}, managed: true,
	}
	f.recs[res] = r
	f.created = append(f.created, p)
	if f.failure != "" {
		f.next[res] = statusFailed
		r.Failure = f.failure
	} else if !f.stuck {
		f.next[res] = statusRunning
	}
	return *r, nil
}

func (f *fakeAPI) transition(verb, resource, via, to string) error {
	if err := f.record(verb); err != nil {
		return err
	}
	r, ok := f.recs[resource]
	if !ok {
		return ErrNotFound
	}
	r.Status = via
	f.next[resource] = to
	return nil
}

func (f *fakeAPI) Start(_ context.Context, r Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.transition("start", r.Resource, "starting", statusRunning)
}

func (f *fakeAPI) Stop(_ context.Context, r Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.transition("stop", r.Resource, "stopping", statusStopped)
}

func (f *fakeAPI) Delete(_ context.Context, r Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.transition("delete", r.Resource, "deleting", "gone")
}

func (f *fakeAPI) Renew(context.Context, Record, time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.renewed++
	return f.record("renew")
}

func (f *fakeAPI) Credential(context.Context, Record) (Credential, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("credential"); err != nil {
		return Credential{}, err
	}
	return Credential{Token: f.tokenOut}, nil
}

// PublishPort publishes a port at a URL built from the sandbox and the port.
func (f *fakeAPI) PublishPort(_ context.Context, r Record, port int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("publish"); err != nil {
		return "", err
	}
	rec, ok := f.recs[r.Resource]
	if !ok {
		return "", ErrNotFound
	}
	url := fmt.Sprintf("https://%d-%s.sbx.example.test", port, resourceID(r.Resource))
	rec.Ports = append(rec.Ports, PublishedPort{Port: port, URL: url})
	f.published = append(f.published, port)
	return url, nil
}

// setStatus puts a sandbox in a status without going through the API.
func (f *fakeAPI) setStatus(name, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.recs {
		if r.Name == name {
			r.Status = status
		}
	}
}

// fakeStarter records the commands of a session and answers them with script.
type fakeStarter struct {
	mu     sync.Mutex
	specs  []ProcessSpec
	stdins []*bytes.Buffer
	script func(spec ProcessSpec, stdin string) []Event
	err    error
	tokens []string
	eps    []Endpoint
	// immediate reports the processes that run their script at once, instead
	// of waiting for the end of their input like a command does.
	immediate func(ProcessSpec) bool
}

func (f *fakeStarter) Start(_ context.Context, ep Endpoint, cred Credential, spec ProcessSpec) (Process, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	f.specs = append(f.specs, spec)
	f.tokens = append(f.tokens, cred.Token)
	f.eps = append(f.eps, ep)
	p := &fakeProcess{spec: spec, script: f.script, closed: make(chan struct{}), eof: make(chan struct{})}
	p.immediate = f.immediate != nil && f.immediate(spec)
	f.stdins = append(f.stdins, &p.stdin)
	return p, nil
}

func (f *fakeStarter) lastSpec(t *testing.T) ProcessSpec {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.specs) == 0 {
		t.Fatal("no process started")
	}
	return f.specs[len(f.specs)-1]
}

func (f *fakeStarter) argvs() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]string, 0, len(f.specs))
	for _, s := range f.specs {
		out = append(out, slices.Clone(s.Argv))
	}
	return out
}

type fakeProcess struct {
	spec    ProcessSpec
	script  func(spec ProcessSpec, stdin string) []Event
	mu      sync.Mutex
	stdin   bytes.Buffer
	eof     chan struct{}
	eofOnce sync.Once
	closed  chan struct{}
	once    sync.Once
	queue   []Event
	started bool
	resizes [][2]uint16
	// immediate skips the wait for the end of the input.
	immediate bool
}

func (p *fakeProcess) Next() (Event, error) {
	if !p.spec.TTY && !p.immediate {
		select {
		case <-p.eof:
		case <-p.closed:
			return Event{}, errors.New("closed")
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.started {
		p.started = true
		if p.script != nil {
			p.queue = p.script(p.spec, p.stdin.String())
		}
		p.queue = append(p.queue, Event{Exited: true})
	}
	ev := p.queue[0]
	p.queue = p.queue[1:]
	return ev, nil
}

func (p *fakeProcess) WriteStdin(b []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stdin.Write(b)
	return nil
}

func (p *fakeProcess) CloseStdin() error {
	p.eofOnce.Do(func() { close(p.eof) })
	return nil
}

func (p *fakeProcess) Resize(rows, cols uint16) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.resizes = append(p.resizes, [2]uint16{rows, cols})
	return nil
}

func (p *fakeProcess) Close() error {
	p.once.Do(func() { close(p.closed) })
	return nil
}

func newTestProvider(t *testing.T) (*Provider, *fakeAPI, *fakeStarter) {
	t.Helper()
	api := newFakeAPI()
	proc := &fakeStarter{}
	p := NewWithClients(api, proc, time.Millisecond, 2*time.Second)
	p.SetRestoredIndexPath(filepath.Join(t.TempDir(), restoredFile))
	return p, api, proc
}
