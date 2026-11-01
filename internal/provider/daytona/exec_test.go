package daytona

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestExecSplitsStdoutAndStderr(t *testing.T) {
	t.Parallel()
	tb := newFakeToolbox(func(ProcessRequest, string) fakeResult {
		return fakeResult{stdout: "out\n", stderr: "err\n"}
	})
	p, api := newTestProvider(t, tb)
	api.add(boxName, statusRunning)
	var stdout, stderr bytes.Buffer
	err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{Command: []string{"echo", "hi"}, Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if stdout.String() != "out\n" || stderr.String() != "err\n" {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if got := tb.reqs[0]; got.Kind != kindExec || !slices.Equal(got.Argv, []string{"echo", "hi"}) || got.Dir != "" {
		t.Fatalf("request = %+v", got)
	}
}

func TestExecReturnsExitError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		res  fakeResult
		code int
	}{
		{"exit code", fakeResult{code: 3}, 3},
		{"signaled", fakeResult{reason: reasonSignaled}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, api := newTestProvider(t, newFakeToolbox(func(ProcessRequest, string) fakeResult { return tt.res }))
			api.add(boxName, statusRunning)
			err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{Command: []string{"false"}})
			exitErr, ok := errors.AsType[*sandbox.ExitError](err)
			if !ok || exitErr.Code != tt.code {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestExecLostProcessIsPlainError(t *testing.T) {
	t.Parallel()
	p, api := newTestProvider(t, newFakeToolbox(func(ProcessRequest, string) fakeResult { return fakeResult{reason: "lost"} }))
	api.add(boxName, statusRunning)
	err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{Command: []string{"sleep", "1"}})
	if err == nil {
		t.Fatal("Exec succeeded")
	}
	if _, ok := errors.AsType[*sandbox.ExitError](err); ok {
		t.Fatalf("lost process reported as ExitError: %v", err)
	}
}

func TestExecEmptyCommand(t *testing.T) {
	t.Parallel()
	p, api := newTestProvider(t, newFakeToolbox(okHandler))
	api.add(boxName, statusRunning)
	if err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{}); err == nil {
		t.Fatal("empty command accepted")
	}
}

func TestExecPipesStdinAndClosesIt(t *testing.T) {
	t.Parallel()
	tb := newFakeToolbox(func(_ ProcessRequest, stdin string) fakeResult {
		return fakeResult{stdout: strings.ToUpper(stdin)}
	})
	p, api := newTestProvider(t, tb)
	api.add(boxName, statusRunning)
	var stdout bytes.Buffer
	err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{
		Command: []string{"cat"}, Stdin: strings.NewReader("hello"), Stdout: &stdout,
	})
	if err != nil || stdout.String() != "HELLO" {
		t.Fatalf("Exec = %v, stdout=%q", err, stdout.String())
	}
	if !tb.reqs[0].Stdin {
		t.Fatal("stdin was not opened")
	}
}

func TestExecInSyncedSandboxUsesWorkspaceAndPath(t *testing.T) {
	t.Parallel()
	tb := newFakeToolbox(okHandler)
	p, api := newTestProvider(t, tb)
	api.add(boxName, statusRunning, sandbox.SyncLabel)
	if err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{Command: []string{"go", "version"}}); err != nil {
		t.Fatal(err)
	}
	req := tb.reqs[0]
	if req.Dir != workspaceDir || req.Argv[0] != "env" || !slices.Contains(req.Argv, "HOME="+daytonaHome) {
		t.Fatalf("request = %+v", req)
	}
	if got := plain(req.Argv); !slices.Equal(got, []string{"go", "version"}) {
		t.Fatalf("command = %v", got)
	}
}

func TestExecStartsStoppedSandbox(t *testing.T) {
	t.Parallel()
	p, api := newTestProvider(t, newFakeToolbox(okHandler))
	api.add("stopped", statusStopped)
	api.add("paused", statusPaused)
	for _, name := range []sandbox.ID{"stopped", "paused"} {
		if err := p.Exec(t.Context(), name, sandbox.ExecOptions{Command: []string{"true"}}); err != nil {
			t.Fatalf("Exec %s: %v", name, err)
		}
	}
	if !api.has("start sb-1") || !api.has("resume sb-2") {
		t.Fatalf("calls = %v", api.calls)
	}
}

func TestExecMissingSandbox(t *testing.T) {
	t.Parallel()
	p, _ := newTestProvider(t, newFakeToolbox(okHandler))
	err := p.Exec(t.Context(), "missing", sandbox.ExecOptions{Command: []string{"true"}})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestExecTTYStreamsAndResizes(t *testing.T) {
	t.Parallel()
	tb := newFakeToolbox(func(ProcessRequest, string) fakeResult { return fakeResult{code: 2} })
	tb.attachOutput = []string{"prompt$ ", "bye\r\n"}
	tb.holdForResize = true
	p, api := newTestProvider(t, tb)
	api.add(boxName, statusRunning)
	resize := make(chan sandbox.TermSize, 1)
	resize <- sandbox.TermSize{Rows: 40, Cols: 120}
	var stdout bytes.Buffer
	err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{
		Command: []string{bashCmd}, Stdin: strings.NewReader("ls\n"), Stdout: &stdout,
		TTY: true, Size: sandbox.TermSize{Rows: 30, Cols: 100}, Resize: resize,
	})
	exitErr, ok := errors.AsType[*sandbox.ExitError](err)
	if !ok || exitErr.Code != 2 {
		t.Fatalf("err = %v", err)
	}
	if stdout.String() != "prompt$ bye\r\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	req := tb.reqs[0]
	if req.Kind != kindPTY || req.Rows != 30 || req.Cols != 100 || !req.Stdin {
		t.Fatalf("request = %+v", req)
	}
	if !slices.Contains(req.Argv, "TERM="+termName) {
		t.Fatalf("argv = %v", req.Argv)
	}
	waitFor(t, func() bool {
		tb.mu.Lock()
		defer tb.mu.Unlock()
		return len(tb.resizes) == 1 && tb.resizes[0] == [2]uint16{40, 120}
	})
	if !tb.attached.closed {
		t.Fatal("attachment was not closed")
	}
}

func TestExecTTYDefaultSize(t *testing.T) {
	t.Parallel()
	tb := newFakeToolbox(okHandler)
	p, api := newTestProvider(t, tb)
	api.add(boxName, statusRunning)
	if err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{Command: []string{bashCmd}, TTY: true}); err != nil {
		t.Fatal(err)
	}
	if tb.reqs[0].Rows != defaultRows || tb.reqs[0].Cols != defaultCols {
		t.Fatalf("size = %dx%d", tb.reqs[0].Rows, tb.reqs[0].Cols)
	}
}

func TestExecHidesAPIKeyInErrors(t *testing.T) {
	t.Parallel()
	tb := newFakeToolbox(okHandler)
	tb.startErr = errors.New("dial https://toolbox.example.test with test-key failed")
	p, api := newTestProvider(t, tb)
	api.add(boxName, statusRunning)
	err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{Command: []string{"true"}})
	if err == nil || strings.Contains(err.Error(), "test-key") {
		t.Fatalf("err = %v", err)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
