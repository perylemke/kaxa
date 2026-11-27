package vercel

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func cmdOf(f *fakeVercel, n int) map[string]any {
	return f.body("POST /v2/sandboxes/sessions/{sid}/cmd", n)
}

func TestExecOutputAndWrapper(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	f.exec = func(*fakeCmd) fakeResult { return fakeResult{stdout: "out\n", stderr: "err\n"} }
	var stdout, stderr bytes.Buffer
	err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{
		Command: []string{"echo", "a b", "$(x)"}, Stdout: &stdout, Stderr: &stderr,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "out\n" || stderr.String() != "err\n" {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
	body := cmdOf(f, 0)
	if body["command"] != shellBash || body["cwd"] != vercelHome || body["wait"] != true || body["logs"] != true {
		t.Fatalf("body = %v", body)
	}
	args := asList(body["args"])
	want := []any{"-c", pathScript, wrapperArg0, toolPath(), "echo", "a b", "$(x)"}
	if len(args) != len(want) {
		t.Fatalf("args = %v", args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args[%d] = %v, want %v", i, args[i], want[i])
		}
	}
	if _, ok := body["attachStdin"]; ok {
		t.Fatal("stdin attached without input")
	}
}

func TestExecExplicitPathSkipsWrapper(t *testing.T) {
	t.Parallel()
	req := runSpec{argv: []string{"npm", "ci"}, dir: "/w", env: []string{"PATH=/x", "A=1", "bad"}}.request()
	if req.Command != "npm" || len(req.Args) != 1 || req.Env["PATH"] != "/x" || req.Env["A"] != "1" || len(req.Env) != 2 {
		t.Fatalf("request = %+v", req)
	}
}

func TestExecDirectories(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		labels []string
		dir    string
		want   string
	}{
		{"home", []string{wrapperArg0}, "", vercelHome},
		{"synced", []string{"kaxa", sandbox.SyncLabel}, "", workspaceDir},
		{"explicit", []string{"kaxa", sandbox.SyncLabel}, "/tmp/../srv/", "/srv"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, f := newTestProvider(t)
			createBox(t, p, func(s *sandbox.Spec) { s.Labels = tt.labels })
			if err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{Command: []string{"pwd"}, Dir: tt.dir}); err != nil {
				t.Fatal(err)
			}
			if got := cmdOf(f, 0)["cwd"]; got != tt.want {
				t.Fatalf("cwd = %v, want %s", got, tt.want)
			}
		})
	}
}

func TestExecRejects(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	ctx := context.Background()
	err := p.Exec(ctx, boxName, sandbox.ExecOptions{Command: []string{"bash"}, TTY: true})
	var unsupported *sandbox.UnsupportedError
	if !errors.As(err, &unsupported) || unsupported.Feature != sandbox.FeatureConsole || !errors.Is(err, sandbox.ErrUnsupported) {
		t.Fatalf("TTY error = %v", err)
	}
	if err := p.Exec(ctx, boxName, sandbox.ExecOptions{}); err == nil {
		t.Fatal("empty command accepted")
	}
	if err := p.Exec(ctx, boxName, sandbox.ExecOptions{Command: []string{"ls"}, Dir: "relative"}); err == nil {
		t.Fatal("relative dir accepted")
	}
	if f.calls("POST /v2/sandboxes/sessions/{sid}/cmd") != 0 {
		t.Fatal("a rejected exec reached the API")
	}
}

func TestExecExitCodes(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		res  fakeResult
		want int
	}{
		{"failure", fakeResult{code: 3}, 3},
		{"signal", fakeResult{code: -9}, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, f := newTestProvider(t)
			createBox(t, p)
			f.exec = func(*fakeCmd) fakeResult { return tt.res }
			err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{Command: []string{"false"}})
			var exit *sandbox.ExitError
			if !errors.As(err, &exit) || exit.Code != tt.want {
				t.Fatalf("error = %v, want exit %d", err, tt.want)
			}
		})
	}
}

func TestExecStreamFailures(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		res  fakeResult
		want string
	}{
		{"no exit code", fakeResult{noExit: true}, "without an exit code"},
		{"stream failure", fakeResult{failure: "disk full"}, "disk full"},
		{"cut stream", fakeResult{cut: true, stdout: "partial"}, "lost the connection"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, f := newTestProvider(t)
			createBox(t, p)
			f.exec = func(*fakeCmd) fakeResult { return tt.res }
			err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{Command: []string{"x"}})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestExecSurvivesWriterFailure(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	f.exec = func(*fakeCmd) fakeResult { return fakeResult{stdout: "a", stderr: "b"} }
	err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{Command: []string{"x"}, Stdout: failWriter{}})
	if err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("error = %v", err)
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestExecStdin(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	var seen string
	f.exec = func(c *fakeCmd) fakeResult {
		f.mu.Lock()
		defer f.mu.Unlock()
		seen = c.stdin.String()
		return fakeResult{stdout: "done"}
	}
	var stdout bytes.Buffer
	input := strings.Repeat("x", stdinChunk+10)
	err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{
		Command: []string{"cat"}, Stdin: strings.NewReader(input), Stdout: &stdout,
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen != input || stdout.String() != "done" {
		t.Fatalf("stdin of %d bytes arrived as %d bytes, stdout %q", len(input), len(seen), stdout.String())
	}
	if cmdOf(f, 0)["attachStdin"] != true {
		t.Fatal("stdin was not attached")
	}
	bodies := f.bodies["POST /v2/sandboxes/sessions/{sid}/cmd/{cid}/stdin"]
	if len(bodies) != 3 || bodies[1]["offset"] != float64(stdinChunk) || bodies[2]["close"] != true {
		t.Fatalf("stdin bodies = %v", bodies)
	}
}

func TestExecStdinReadError(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	f.exec = func(*fakeCmd) fakeResult { return fakeResult{} }
	err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{
		Command: []string{"cat"}, Stdin: io.MultiReader(strings.NewReader("hi"), errReader{}),
	})
	if err != nil {
		t.Fatal(err)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("broken input") }

type stdinFailClient struct {
	apiClient
	calls int
}

func (c *stdinFailClient) WriteStdin(context.Context, string, string, []byte, int64, bool) error {
	c.calls++
	return errors.New("stdin refused")
}

func TestPumpInputStopsOnWriteFailure(t *testing.T) {
	t.Parallel()
	c := &stdinFailClient{}
	s := &session{api: c, rec: Record{SessionID: "s"}}
	s.pumpInput(context.Background(), "c", strings.NewReader("hi"))
	if c.calls != 1 {
		t.Fatalf("WriteStdin calls = %d, want 1", c.calls)
	}
}

func TestExecKillsOnCancel(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	f.exec = func(*fakeCmd) fakeResult { return fakeResult{block: true} }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Exec(ctx, boxName, sandbox.ExecOptions{Command: []string{"sleep", "99"}}) }()
	for f.calls("POST /v2/sandboxes/sessions/{sid}/cmd") == 0 {
		time.Sleep(time.Millisecond)
	}
	// Give the stream time to deliver the start event that names the command.
	time.Sleep(100 * time.Millisecond)
	cancel()
	if err := <-done; err == nil {
		t.Fatal("canceled exec returned no error")
	}
	select {
	case <-f.killedID:
	case <-time.After(5 * time.Second):
		t.Fatal("canceled command was not killed")
	}
}

func TestExecAPIFailure(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	f.fail2("POST /v2/sandboxes/sessions/{sid}/cmd", 410)
	if err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{Command: []string{"x"}}); err == nil {
		t.Fatal("API failure ignored")
	}
}

func TestCapture(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	s, err := p.open(context.Background(), boxName)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name    string
		res     fakeResult
		wantOut string
		wantErr string
	}{
		{"ok", fakeResult{stdout: "hello"}, "hello", ""},
		{"exit with detail", fakeResult{code: 2, stderr: " boom \n"}, "", "exit status 2: boom"},
		{"exit without detail", fakeResult{code: 4}, "", "exit status 4"},
		{"stream failure with detail", fakeResult{failure: "lost", stderr: "ctx"}, "", "lost: ctx"},
		{"stream failure", fakeResult{failure: "lost"}, "", "lost"},
	} {
		f.exec = func(*fakeCmd) fakeResult { return tt.res }
		out, err := s.capture(context.Background(), "/w", []string{"A=1"}, []string{"x"}, nil)
		if out != tt.wantOut || (tt.wantErr == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), tt.wantErr)) {
			t.Errorf("%s: capture() = %q, %v", tt.name, out, err)
		}
	}
}
