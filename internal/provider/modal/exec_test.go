package modal_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/provider/modal"
	"github.com/perylemke/kaxa/internal/sandbox"
)

const (
	testShell        = "bash"
	testKaxa         = "kaxa"
	testLongList     = "-la"
	testProviderName = "modal"
)

func runningFake(t *testing.T) *fakeClient {
	t.Helper()
	f := newFakeClient()
	f.boxes[testName] = &modal.Record{ID: testSandboxID, Name: testName}
	return f
}

func TestExecRoutesOutputAndStdin(t *testing.T) {
	t.Parallel()

	f := runningFake(t)
	f.process = newFakeProcess("out\n", "err\n", 0)
	var stdout, stderr lockedBuffer

	err := newProvider(t, f).Exec(context.Background(), testName, sandbox.ExecOptions{
		Command: []string{"cat"},
		Stdin:   strings.NewReader("input"),
		Stdout:  &stdout,
		Stderr:  &stderr,
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if stdout.String() != "out\n" || stderr.String() != "err\n" {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
	if got := f.process.stdin.String(); got != "input" {
		t.Fatalf("stdin = %q", got)
	}
	if f.execName != testName || !slices.Equal(f.execArgv, []string{"cat"}) || f.execPTY {
		t.Fatalf("exec = %q %v pty=%v", f.execName, f.execArgv, f.execPTY)
	}
	if !f.process.closed {
		t.Fatal("process handle was not closed")
	}
}

func TestExecNilStdinClosesStdin(t *testing.T) {
	t.Parallel()

	f := runningFake(t)
	f.process = newFakeProcess("", "", 0)

	// Wait blocks until stdin is closed, so returning proves it was closed.
	if err := newProvider(t, f).Exec(context.Background(), testName, sandbox.ExecOptions{Command: []string{"true"}}); err != nil {
		t.Fatalf("exec: %v", err)
	}
}

func TestExecNonZeroExitIsExitError(t *testing.T) {
	t.Parallel()

	f := runningFake(t)
	f.process = newFakeProcess("", "", 3)

	err := newProvider(t, f).Exec(context.Background(), testName, sandbox.ExecOptions{Command: []string{"false"}})
	exitErr, ok := errors.AsType[*sandbox.ExitError](err)
	if !ok || exitErr.Code != 3 {
		t.Fatalf("err = %v, want exit code 3", err)
	}
}

func TestExecTTYAppliesInitialSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		opts     sandbox.ExecOptions
		wantArgv []string
		wantPTY  bool
	}{
		{
			name:     "tty with size",
			opts:     sandbox.ExecOptions{Command: []string{testShell}, TTY: true, Size: sandbox.TermSize{Rows: 40, Cols: 120}},
			wantArgv: []string{"sh", "-c", `stty rows "$1" cols "$2" 2>/dev/null; shift 2; exec "$@"`, testKaxa, "40", "120", testShell},
			wantPTY:  true,
		},
		{
			name:     "tty without size",
			opts:     sandbox.ExecOptions{Command: []string{testShell}, TTY: true},
			wantArgv: []string{testShell},
			wantPTY:  true,
		},
		{
			name:     "no tty ignores size",
			opts:     sandbox.ExecOptions{Command: []string{"ls", testLongList}, Size: sandbox.TermSize{Rows: 40, Cols: 120}},
			wantArgv: []string{"ls", testLongList},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := runningFake(t)
			if err := newProvider(t, f).Exec(context.Background(), testName, tt.opts); err != nil {
				t.Fatalf("exec: %v", err)
			}
			if !slices.Equal(f.execArgv, tt.wantArgv) || f.execPTY != tt.wantPTY {
				t.Fatalf("argv = %q pty = %v, want %q pty = %v", f.execArgv, f.execPTY, tt.wantArgv, tt.wantPTY)
			}
		})
	}
}

func TestExecTTYSkipsStderrAndDrainsResize(t *testing.T) {
	t.Parallel()

	f := runningFake(t)
	f.process = newFakeProcess("shell", "never read", 0)
	resize := make(chan sandbox.TermSize) // unbuffered: a send blocks unless drained
	var stdout, stderr lockedBuffer
	go func() { resize <- sandbox.TermSize{Rows: 1, Cols: 1} }()

	err := newProvider(t, f).Exec(context.Background(), testName, sandbox.ExecOptions{
		Command: []string{testShell}, TTY: true, Stdout: &stdout, Stderr: &stderr, Resize: resize,
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if stdout.String() != "shell" || stderr.String() != "" {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestExecRejectsBadInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      sandbox.ID
		command []string
		mutate  func(*fakeClient)
		wantNot bool
	}{
		{name: "invalid name", id: "Bad Name", command: []string{"ls"}},
		{name: "empty command", id: testName},
		{name: "not found", id: "other", command: []string{"ls"}, wantNot: true},
		{
			name: "finished sandbox", id: testName, command: []string{"ls"}, wantNot: true,
			mutate: func(f *fakeClient) {
				code := 0
				f.boxes[testName].ExitCode = &code
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := runningFake(t)
			if tt.mutate != nil {
				tt.mutate(f)
			}
			err := newProvider(t, f).Exec(context.Background(), tt.id, sandbox.ExecOptions{Command: tt.command})
			if err == nil {
				t.Fatal("exec succeeded")
			}
			if tt.wantNot && !errors.Is(err, modal.ErrNotFound) {
				t.Fatalf("err = %v, want not found", err)
			}
			if f.execArgv != nil {
				t.Fatalf("command was started: %v", f.execArgv)
			}
		})
	}
}

func TestExecWithoutCredentials(t *testing.T) {
	t.Parallel()

	err := modal.New().Exec(context.Background(), testName, sandbox.ExecOptions{Command: []string{"ls"}})
	if !errors.Is(err, modal.ErrMissingCredentials) {
		t.Fatalf("err = %v", err)
	}
}

func TestExecRedactsSecrets(t *testing.T) {
	t.Parallel()

	f := runningFake(t)
	f.execErr = errors.New("auth failed for " + testTokenSecret)

	err := newProvider(t, f).Exec(context.Background(), testName, sandbox.ExecOptions{Command: []string{"ls"}})
	if err == nil || strings.Contains(err.Error(), testTokenSecret) {
		t.Fatalf("err = %v, want the secret redacted", err)
	}
}

func TestExecStopsWhenContextIsCancelled(t *testing.T) {
	t.Parallel()

	f := runningFake(t)
	f.process = newFakeProcess("", "", 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Stdin never ends, so only the cancelled context can end Wait.
	stdin := blockingReader{}
	err := newProvider(t, f).Exec(ctx, testName, sandbox.ExecOptions{Command: []string{"sleep"}, Stdin: stdin})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

type blockingReader struct{}

func (blockingReader) Read([]byte) (int, error) { select {} }
