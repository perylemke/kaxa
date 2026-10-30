package e2b_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"

	"github.com/perylemke/kaxa/internal/provider/e2b"
)

const (
	testKey    = "e2b_secret_key"
	testToken  = "daemon-token-123"
	testBox    = "web"
	stateReady = "running"
	testKaxa   = "kaxa"
	testUser   = "user"
	testApp    = "app"
	testBig    = "big"
	testTeam   = "team"
	probeName  = "kaxa"
)

// fakeAPI is an in-memory E2B control plane.
type fakeAPI struct {
	mu      sync.Mutex
	boxes   map[string]*e2b.Record
	nextID  int
	creates []e2b.CreateParams
	killed  []string
	paused  []pauseCall
	connect []connectCall
	// failWith makes every call fail.
	failWith   error
	pauseErr   error
	createErr  error
	connectErr error
}

type pauseCall struct {
	id     string
	memory bool
}

type connectCall struct {
	id     string
	reboot bool
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{boxes: map[string]*e2b.Record{}}
}

func (f *fakeAPI) add(name, state string, labels []string, apiPort int) *e2b.Record {
	f.nextID++
	rec := &e2b.Record{ID: fmt.Sprintf("sbx-%d", f.nextID), Name: name, State: state, Labels: labels, APIPort: apiPort}
	f.boxes[name] = rec
	return rec
}

func (f *fakeAPI) Create(_ context.Context, p e2b.CreateParams) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWith != nil {
		return "", f.failWith
	}
	if f.createErr != nil {
		return "", f.createErr
	}
	f.creates = append(f.creates, p)
	rec := f.add(p.Name, stateReady, p.Labels, p.APIPort)
	rec.Template = p.Template
	return rec.ID, nil
}

func (f *fakeAPI) List(context.Context) ([]e2b.Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWith != nil {
		return nil, f.failWith
	}
	out := make([]e2b.Record, 0, len(f.boxes))
	for _, r := range f.boxes {
		out = append(out, *r)
	}
	return out, nil
}

func (f *fakeAPI) Find(_ context.Context, name string) (e2b.Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWith != nil {
		return e2b.Record{}, f.failWith
	}
	r, ok := f.boxes[name]
	if !ok {
		return e2b.Record{}, e2b.ErrNotFound
	}
	return *r, nil
}

func (f *fakeAPI) Kill(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWith != nil {
		return f.failWith
	}
	for name, r := range f.boxes {
		if r.ID == id {
			delete(f.boxes, name)
			f.killed = append(f.killed, id)
			return nil
		}
	}
	return e2b.ErrNotFound
}

func (f *fakeAPI) Pause(_ context.Context, id string, memory bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pauseErr != nil {
		return f.pauseErr
	}
	f.paused = append(f.paused, pauseCall{id, memory})
	for _, r := range f.boxes {
		if r.ID == id {
			r.State = "paused"
		}
	}
	return nil
}

func (f *fakeAPI) Connect(_ context.Context, id string, p e2b.ConnectParams) (e2b.Connection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.connectErr != nil {
		return e2b.Connection{}, f.connectErr
	}
	f.connect = append(f.connect, connectCall{id, p.Reboot})
	for _, r := range f.boxes {
		if r.ID == id {
			r.State = stateReady
		}
	}
	return e2b.Connection{ID: id, Domain: "e2b.app", AccessToken: testToken}, nil
}

// fakeEnvd is an in-memory sandbox daemon. script decides what each command does.
type fakeEnvd struct {
	mu      sync.Mutex
	starts  []e2b.StartRequest
	uploads []upload
	procs   []*fakeProc
	// script returns the process for a command. The default succeeds with no output.
	script    func(req e2b.StartRequest) *fakeProc
	startErr  error
	uploadErr error
}

type upload struct {
	user string
	path string
	body string
}

func (f *fakeEnvd) Start(_ context.Context, _ e2b.Connection, req e2b.StartRequest) (e2b.Process, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return nil, f.startErr
	}
	f.starts = append(f.starts, req)
	p := exited(0)
	if f.script != nil {
		p = f.script(req)
	}
	f.procs = append(f.procs, p)
	return p, nil
}

func (f *fakeEnvd) Upload(_ context.Context, _ e2b.Connection, user, path string, body io.Reader) error {
	raw, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.uploadErr != nil {
		return f.uploadErr
	}
	f.uploads = append(f.uploads, upload{user, path, string(raw)})
	return nil
}

// argvs returns every command started, as "cmd arg arg".
func (f *fakeEnvd) argvs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.starts))
	for _, s := range f.starts {
		out = append(out, strings.Join(append([]string{s.Cmd}, s.Args...), " "))
	}
	return out
}

// fakeProc is a command with a scripted list of events.
type fakeProc struct {
	mu     sync.Mutex
	events []e2b.Event
	// block makes Next wait after the events, until Close.
	block    chan struct{}
	nextErr  error
	input    bytes.Buffer
	stdinEnd bool
	resizes  [][2]uint16
	inputErr error
	closed   bool
}

func exited(code int) *fakeProc {
	return &fakeProc{events: []e2b.Event{{Kind: e2b.EventEnd, ExitCode: code}}}
}

func output(stdout, stderr string, code int) *fakeProc {
	p := &fakeProc{}
	if stdout != "" {
		p.events = append(p.events, e2b.Event{Kind: e2b.EventStdout, Data: []byte(stdout)})
	}
	if stderr != "" {
		p.events = append(p.events, e2b.Event{Kind: e2b.EventStderr, Data: []byte(stderr)})
	}
	p.events = append(p.events, e2b.Event{Kind: e2b.EventEnd, ExitCode: code})
	return p
}

func (p *fakeProc) Next() (e2b.Event, error) {
	for {
		p.mu.Lock()
		if len(p.events) > 0 {
			ev := p.events[0]
			p.events = p.events[1:]
			p.mu.Unlock()
			return ev, nil
		}
		err, block := p.nextErr, p.block
		p.mu.Unlock()
		if block == nil {
			if err != nil {
				return e2b.Event{}, err
			}
			return e2b.Event{}, io.EOF
		}
		<-block
		// The test that released block may have queued more events.
		p.mu.Lock()
		more := len(p.events) > 0
		p.block = nil
		p.mu.Unlock()
		if !more {
			if err != nil {
				return e2b.Event{}, err
			}
			return e2b.Event{}, io.EOF
		}
	}
}

func (p *fakeProc) SendInput(_ context.Context, data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.inputErr != nil {
		return p.inputErr
	}
	p.input.Write(data)
	return nil
}

func (p *fakeProc) CloseStdin(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stdinEnd = true
	return nil
}

func (p *fakeProc) Resize(_ context.Context, rows, cols uint16) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.resizes = append(p.resizes, [2]uint16{rows, cols})
	return nil
}

func (p *fakeProc) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed && p.block != nil {
		close(p.block)
	}
	p.closed = true
}

func (p *fakeProc) received() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.input.String()
}

func (p *fakeProc) stdinClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stdinEnd
}

func (p *fakeProc) resized() [][2]uint16 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.resizes)
}

// newProvider returns a provider over fakes with one sandbox called testBox.
func newProvider(labels []string, apiPort int) (*e2b.Provider, *fakeAPI, *fakeEnvd) {
	api := newFakeAPI()
	api.add(testBox, stateReady, labels, apiPort)
	envd := &fakeEnvd{}
	return e2b.NewWithClient(api, envd, testKey), api, envd
}
