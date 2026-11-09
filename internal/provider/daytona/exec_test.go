package daytona

import (
	"bytes"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestExecSplitsStdoutAndStderr(t *testing.T) {
	t.Parallel()
	tb := newFakeToolbox(func(runSpec, string) fakeResult {
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
	if got := tb.reqs[0]; !slices.Equal(got.argv, []string{"echo", "hi"}) || got.dir != "" || got.tty {
		t.Fatalf("request = %+v", got)
	}
}

func TestExecReturnsExitError(t *testing.T) {
	t.Parallel()
	p, api := newTestProvider(t, newFakeToolbox(func(runSpec, string) fakeResult { return fakeResult{code: 3} }))
	api.add(boxName, statusRunning)
	err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{Command: []string{"false"}})
	exitErr, ok := errors.AsType[*sandbox.ExitError](err)
	if !ok || exitErr.Code != 3 {
		t.Fatalf("err = %v", err)
	}
}

func TestExecLostProcessIsPlainError(t *testing.T) {
	t.Parallel()
	p, api := newTestProvider(t, newFakeToolbox(func(runSpec, string) fakeResult { return fakeResult{err: errors.New("sandbox lost")} }))
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
	tb := newFakeToolbox(func(_ runSpec, stdin string) fakeResult {
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
	if tb.stdins[0] != "hello" {
		t.Fatalf("stdin = %q", tb.stdins[0])
	}
}

func TestExecRequestedDirOverridesWorkspace(t *testing.T) {
	t.Parallel()
	tb := newFakeToolbox(okHandler)
	p, api := newTestProvider(t, tb)
	api.add(boxName, statusRunning, sandbox.SyncLabel)
	err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{
		Command: []string{"go", "test"}, Dir: "/home/daytona/workspace/app/",
	})
	if err != nil {
		t.Fatal(err)
	}
	if tb.reqs[0].dir != "/home/daytona/workspace/app" {
		t.Fatalf("dir = %q", tb.reqs[0].dir)
	}
}

func TestExecRejectsRelativeDir(t *testing.T) {
	t.Parallel()
	tb := newFakeToolbox(okHandler)
	p, _ := newTestProvider(t, tb)
	err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{Command: []string{"ls"}, Dir: "app"})
	if err == nil || len(tb.reqs) != 0 {
		t.Fatalf("err = %v reqs = %d", err, len(tb.reqs))
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
	if req.dir != workspaceDir || !slices.Contains(req.env, "HOME="+daytonaHome) {
		t.Fatalf("request = %+v", req)
	}
	if !slices.Equal(req.argv, []string{"go", "version"}) {
		t.Fatalf("command = %v", req.argv)
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
	tb := newFakeToolbox(func(runSpec, string) fakeResult { return fakeResult{code: 2} })
	tb.ttyOutput = []string{"prompt$ ", "bye\r\n"}
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
	if !req.tty || req.size.Rows != 30 || req.size.Cols != 100 || req.stdin == nil {
		t.Fatalf("request = %+v", req)
	}
	expect(t, "resizes", tb.resizes, [][2]uint16{{40, 120}})
}

func TestTermSizeDefault(t *testing.T) {
	t.Parallel()
	rows, cols := termSize(sandbox.TermSize{})
	expect(t, "default size", []uint16{rows, cols}, []uint16{defaultRows, defaultCols})
	rows, cols = termSize(sandbox.TermSize{Rows: 30, Cols: 100})
	expect(t, "given size", []uint16{rows, cols}, []uint16{30, 100})
}

func TestExecHidesAPIKeyInErrors(t *testing.T) {
	t.Parallel()
	tb := newFakeToolbox(okHandler)
	tb.startErr = errors.New("dial https://proxy.example.test with test-key failed")
	p, api := newTestProvider(t, tb)
	api.add(boxName, statusRunning)
	err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{Command: []string{"true"}})
	if err == nil || strings.Contains(err.Error(), "test-key") {
		t.Fatalf("err = %v", err)
	}
}

func TestPipedInputSkipsCharacterDevices(t *testing.T) {
	t.Parallel()
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = null.Close() })
	if pipedInput(null) != nil {
		t.Fatal("/dev/null was kept as input")
	}
	file, err := os.Open("exec_test.go")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	if pipedInput(file) == nil {
		t.Fatal("a regular file was dropped")
	}
	reader := strings.NewReader("x")
	if pipedInput(reader) != reader {
		t.Fatal("a reader was changed")
	}
}
