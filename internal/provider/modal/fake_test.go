package modal_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"

	"github.com/perylemke/kaxa/internal/provider/modal"
)

// fakeClient is an in-memory modal SDK client.
type fakeClient struct {
	boxes    map[string]*modal.Record
	creates  int
	failWith error

	// Exec bookkeeping: what the provider asked for and what the process does.
	execName string
	execArgv []string
	execPTY  bool
	execDir  string
	execErr  error
	process  *fakeProcess
	// execs records every command, in order, for tests that run several.
	execs []execCall
}

type execCall struct {
	argv []string
	dir  string
}

func newFakeClient() *fakeClient {
	return &fakeClient{boxes: map[string]*modal.Record{}}
}

func (f *fakeClient) Create(_ context.Context, name string, labels []string) (modal.Record, error) {
	f.creates++
	if f.failWith != nil {
		return modal.Record{}, f.failWith
	}
	if _, ok := f.boxes[name]; ok {
		return modal.Record{}, modal.ErrAlreadyExists
	}
	rec := &modal.Record{ID: "sb-" + name, Name: name, Labels: labels}
	f.boxes[name] = rec
	return *rec, nil
}

func (f *fakeClient) List(context.Context) ([]modal.Record, error) {
	if f.failWith != nil {
		return nil, f.failWith
	}
	out := make([]modal.Record, 0, len(f.boxes))
	for _, rec := range f.boxes {
		out = append(out, *rec)
	}
	return out, nil
}

func (f *fakeClient) Get(_ context.Context, name string) (modal.Record, error) {
	if f.failWith != nil {
		return modal.Record{}, f.failWith
	}
	rec, ok := f.boxes[name]
	if !ok {
		return modal.Record{}, modal.ErrNotFound
	}
	return *rec, nil
}

func (f *fakeClient) Terminate(_ context.Context, name string) error {
	if f.failWith != nil {
		return f.failWith
	}
	if _, ok := f.boxes[name]; !ok {
		return modal.ErrNotFound
	}
	delete(f.boxes, name)
	return nil
}

func (f *fakeClient) Close() {}

func (f *fakeClient) Exec(_ context.Context, name string, argv []string, params modal.ExecParams) (modal.Process, error) {
	f.execName, f.execArgv, f.execPTY, f.execDir = name, argv, params.PTY, params.Workdir
	f.execs = append(f.execs, execCall{argv: argv, dir: params.Workdir})
	if f.execErr != nil {
		return nil, f.execErr
	}
	if f.process == nil {
		f.process = newFakeProcess("", "", 0)
	}
	return f.process, nil
}

// fakeProcess is a scripted command. Wait blocks until stdin is closed, as a
// real command reading stdin would, so tests can read stdin without a race.
type fakeProcess struct {
	stdin    *lockedBuffer
	stdinEnd chan struct{}
	once     sync.Once
	stdout   io.Reader
	stderr   io.Reader
	code     int
	waitErr  error
	closed   bool
}

func newFakeProcess(stdout, stderr string, code int) *fakeProcess {
	return &fakeProcess{
		stdin:    &lockedBuffer{},
		stdinEnd: make(chan struct{}),
		stdout:   strings.NewReader(stdout),
		stderr:   strings.NewReader(stderr),
		code:     code,
	}
}

func (p *fakeProcess) Stdin() io.WriteCloser { return stdinCloser{p} }
func (p *fakeProcess) Stdout() io.Reader     { return p.stdout }
func (p *fakeProcess) Stderr() io.Reader     { return p.stderr }
func (p *fakeProcess) Close()                { p.closed = true }

func (p *fakeProcess) Wait(ctx context.Context) (int, error) {
	select {
	case <-p.stdinEnd:
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	return p.code, p.waitErr
}

type stdinCloser struct{ p *fakeProcess }

func (s stdinCloser) Write(b []byte) (int, error) { return s.p.stdin.Write(b) }
func (s stdinCloser) Close() error {
	s.p.once.Do(func() { close(s.p.stdinEnd) })
	return nil
}

// lockedBuffer is a bytes.Buffer safe for concurrent use.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}
