package e2b_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/provider/e2b"
	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestExecRunsArgvWithoutShell(t *testing.T) {
	t.Parallel()

	p, api, envd := newProvider([]string{testKaxa}, 0)
	envd.script = func(e2b.StartRequest) *fakeProc { return output("hello\n", "warn\n", 0) }
	var stdout, stderr bytes.Buffer

	err := p.Exec(context.Background(), testBox, sandbox.ExecOptions{
		Command: []string{"echo", "a; rm -rf /", "$HOME"}, Stdout: &stdout, Stderr: &stderr,
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if stdout.String() != "hello\n" || stderr.String() != "warn\n" {
		t.Fatalf("stdout = %q stderr = %q", stdout.String(), stderr.String())
	}
	if len(envd.starts) != 1 {
		t.Fatalf("starts = %+v", envd.starts)
	}
	req := envd.starts[0]
	if req.Cmd != "echo" || !slices.Equal(req.Args, []string{"a; rm -rf /", "$HOME"}) || req.Cwd != "" || req.User != testUser || req.PTY || req.Stdin {
		t.Fatalf("request = %+v", req)
	}
	if len(api.connect) != 1 || api.connect[0].reboot {
		t.Fatalf("connect calls = %+v, want one plain resume", api.connect)
	}
}

func TestExecInSyncedSandboxUsesWorkspaceAndToolchainPath(t *testing.T) {
	t.Parallel()

	p, _, envd := newProvider([]string{testKaxa, sandbox.SyncLabel}, 0)

	if err := p.Exec(context.Background(), testBox, sandbox.ExecOptions{Command: []string{"uv", "run", "pytest"}}); err != nil {
		t.Fatalf("exec: %v", err)
	}
	req := envd.starts[0]
	if req.Cmd != "env" || req.Cwd != "/home/user/workspace" {
		t.Fatalf("request = %+v", req)
	}
	tail := req.Args[len(req.Args)-3:]
	if !slices.Equal(tail, []string{"uv", "run", "pytest"}) {
		t.Fatalf("args = %v", req.Args)
	}
	for _, want := range []string{"HOME=/home/user", "USER=user"} {
		if !slices.Contains(req.Args, want) {
			t.Fatalf("args = %v, missing %s", req.Args, want)
		}
	}
	if !slices.ContainsFunc(req.Args, func(a string) bool { return strings.HasPrefix(a, "PATH=/home/user/.local/bin:") }) {
		t.Fatalf("args = %v, missing the toolchain PATH", req.Args)
	}
}

func TestExecExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		code     int
		wantCode int
	}{
		{name: "plain failure", code: 2, wantCode: 2},
		{name: "signal", code: -1, wantCode: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, _, envd := newProvider(nil, 0)
			envd.script = func(e2b.StartRequest) *fakeProc { return exited(tt.code) }
			err := p.Exec(context.Background(), testBox, sandbox.ExecOptions{Command: []string{"false"}})
			exitErr, ok := errors.AsType[*sandbox.ExitError](err)
			if !ok || exitErr.Code != tt.wantCode {
				t.Fatalf("err = %v, want exit %d", err, tt.wantCode)
			}
		})
	}
}

func TestExecCommandThatCouldNotRun(t *testing.T) {
	t.Parallel()

	p, _, envd := newProvider(nil, 0)
	envd.script = func(e2b.StartRequest) *fakeProc {
		return &fakeProc{events: []e2b.Event{{Kind: e2b.EventEnd, Err: "fork/exec nope: no such file"}}}
	}
	err := p.Exec(context.Background(), testBox, sandbox.ExecOptions{Command: []string{"nope"}})
	if err == nil || !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("err = %v", err)
	}
	if _, isExit := errors.AsType[*sandbox.ExitError](err); isExit {
		t.Fatal("a command that could not run must not look like an exit code")
	}
}

func TestExecStdinIsSentAndClosed(t *testing.T) {
	t.Parallel()

	p, _, envd := newProvider(nil, 0)
	var proc *fakeProc
	envd.script = func(e2b.StartRequest) *fakeProc {
		proc = &fakeProc{block: make(chan struct{})}
		go func() {
			// End the command once stdin is closed, as cat would.
			for !proc.stdinClosed() {
				time.Sleep(time.Millisecond)
			}
			proc.mu.Lock()
			proc.events = append(proc.events, e2b.Event{Kind: e2b.EventEnd})
			close(proc.block)
			proc.block = nil
			proc.mu.Unlock()
		}()
		return proc
	}

	err := p.Exec(context.Background(), testBox, sandbox.ExecOptions{Command: []string{"cat"}, Stdin: strings.NewReader("piped data")})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if got := proc.received(); got != "piped data" {
		t.Fatalf("stdin = %q", got)
	}
	if !envd.starts[0].Stdin {
		t.Fatal("stdin was not attached")
	}
}

func TestExecTTYSizesAndResizes(t *testing.T) {
	t.Parallel()

	p, _, envd := newProvider(nil, 0)
	resize := make(chan sandbox.TermSize, 1)
	resize <- sandbox.TermSize{Rows: 50, Cols: 200}
	var proc *fakeProc
	envd.script = func(e2b.StartRequest) *fakeProc {
		proc = &fakeProc{block: make(chan struct{})}
		go func() {
			for len(proc.resized()) == 0 {
				time.Sleep(time.Millisecond)
			}
			proc.mu.Lock()
			proc.events = append(proc.events,
				e2b.Event{Kind: e2b.EventPTY, Data: []byte("$ ")},
				e2b.Event{Kind: e2b.EventEnd})
			close(proc.block)
			proc.block = nil
			proc.mu.Unlock()
		}()
		return proc
	}
	var stdout bytes.Buffer

	err := p.Exec(context.Background(), testBox, sandbox.ExecOptions{
		Command: []string{"bash"}, TTY: true, Size: sandbox.TermSize{Rows: 30, Cols: 100},
		Stdin: strings.NewReader("ls\n"), Stdout: &stdout, Resize: resize,
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	req := envd.starts[0]
	if !req.PTY || req.Rows != 30 || req.Cols != 100 || req.Env["TERM"] == "" {
		t.Fatalf("request = %+v", req)
	}
	if got := proc.resized(); len(got) != 1 || got[0] != [2]uint16{50, 200} {
		t.Fatalf("resizes = %v", got)
	}
	if stdout.String() != "$ " {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if proc.stdinClosed() {
		t.Fatal("a terminal's stdin must not be closed")
	}
}

func TestExecTTYDefaultSize(t *testing.T) {
	t.Parallel()

	p, _, envd := newProvider(nil, 0)
	if err := p.Exec(context.Background(), testBox, sandbox.ExecOptions{Command: []string{"bash"}, TTY: true}); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if req := envd.starts[0]; req.Rows != 24 || req.Cols != 80 {
		t.Fatalf("size = %dx%d", req.Rows, req.Cols)
	}
}

func TestExecErrors(t *testing.T) {
	t.Parallel()

	t.Run("empty command", func(t *testing.T) {
		t.Parallel()
		p, _, _ := newProvider(nil, 0)
		if err := p.Exec(context.Background(), testBox, sandbox.ExecOptions{}); err == nil {
			t.Fatal("empty command accepted")
		}
	})
	t.Run("unknown sandbox", func(t *testing.T) {
		t.Parallel()
		p, _, _ := newProvider(nil, 0)
		err := p.Exec(context.Background(), "missing", sandbox.ExecOptions{Command: []string{"ls"}})
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("resume fails", func(t *testing.T) {
		t.Parallel()
		p, api, envd := newProvider(nil, 0)
		api.connectErr = errors.New("resume failed for " + testKey)
		err := p.Exec(context.Background(), testBox, sandbox.ExecOptions{Command: []string{"ls"}})
		if err == nil || strings.Contains(err.Error(), testKey) || len(envd.starts) != 0 {
			t.Fatalf("err = %v starts = %d", err, len(envd.starts))
		}
	})
	t.Run("daemon error hides the token", func(t *testing.T) {
		t.Parallel()
		p, _, envd := newProvider(nil, 0)
		envd.startErr = errors.New("bad token " + testToken)
		err := p.Exec(context.Background(), testBox, sandbox.ExecOptions{Command: []string{"ls"}})
		if err == nil || strings.Contains(err.Error(), testToken) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("stream cut off", func(t *testing.T) {
		t.Parallel()
		p, _, envd := newProvider(nil, 0)
		envd.script = func(e2b.StartRequest) *fakeProc { return &fakeProc{nextErr: io.ErrUnexpectedEOF} }
		err := p.Exec(context.Background(), testBox, sandbox.ExecOptions{Command: []string{"sleep", "9"}})
		if err == nil || !strings.Contains(err.Error(), "lost the connection") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestExecKeepsDrainingWhenOutputFails(t *testing.T) {
	t.Parallel()

	p, _, envd := newProvider(nil, 0)
	envd.script = func(e2b.StartRequest) *fakeProc { return output("data", "", 0) }
	err := p.Exec(context.Background(), testBox, sandbox.ExecOptions{Command: []string{"ls"}, Stdout: failWriter{}})
	if err == nil || !strings.Contains(err.Error(), "closed pipe") {
		t.Fatalf("err = %v", err)
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
