package blaxel

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestQuoteArgvStaysOneArgument(t *testing.T) {
	t.Parallel()
	arg := "a b'$(echo pwned);echo"
	line := quoteArgv([]string{"printf", "%s", arg})
	if line != `'printf' '%s' 'a b'\''$(echo pwned);echo'` {
		t.Fatalf("line = %q", line)
	}
	// The remote process API receives one shell string. This check runs that
	// string locally and proves the metacharacters stay inside one argument.
	out, err := exec.Command("sh", "-c", line).Output() //gosec:disable G204 -- the command is the quoted argv under test
	if err != nil || string(out) != arg {
		t.Fatalf("printf = %q, %v", out, err)
	}
}

func TestExecSplitsStdoutAndStderr(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	api.handler = func(runSpec) fakeResult { return fakeResult{stdout: "out\n", stderr: "err\n"} }
	p := newTestProvider(t, api)
	api.add(boxName, statusRunning)
	var stdout, stderr bytes.Buffer
	err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{Command: []string{"echo", "hi"}, Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if stdout.String() != "out\n" || stderr.String() != "err\n" {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if !commandIs(api.argvs()[0], "echo", "hi") || api.dirs[0] != blaxelHome {
		t.Fatalf("argv = %v dir = %q", api.argvs()[0], api.dirs[0])
	}
	if !slices.Contains(api.envs[0], "HOME="+blaxelHome) || !slices.Contains(api.envs[0], "USER="+blaxelUser) {
		t.Fatalf("env = %v", api.envs[0])
	}
}

func TestExecReturnsExitError(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	api.handler = func(runSpec) fakeResult { return fakeResult{code: 3} }
	p := newTestProvider(t, api)
	api.add(boxName, statusRunning)
	err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{Command: []string{"false"}})
	exitErr, ok := errors.AsType[*sandbox.ExitError](err)
	if !ok || exitErr.Code != 3 {
		t.Fatalf("err = %v", err)
	}
}

func TestExecLostProcessIsPlainError(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	api.handler = func(runSpec) fakeResult { return fakeResult{err: errors.New("sandbox lost")} }
	p := newTestProvider(t, api)
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
	api := newFakeAPI()
	p := newTestProvider(t, api)
	api.add(boxName, statusRunning)
	if err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{}); err == nil {
		t.Fatal("empty command accepted")
	}
	if len(api.argvs()) != 0 {
		t.Fatal("empty command ran")
	}
}

func TestExecPipesStdin(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
	api.add(boxName, statusRunning)
	err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{Command: []string{"cat"}, Stdin: strings.NewReader("hello")})
	if err != nil {
		t.Fatal(err)
	}
	if api.stdins[0] != "hello" {
		t.Fatalf("stdin = %q", api.stdins[0])
	}
}

func TestExecInSyncedSandboxUsesWorkspace(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
	api.add(boxName, statusRunning, sandbox.SyncLabel)
	if err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{Command: []string{"go", "version"}}); err != nil {
		t.Fatal(err)
	}
	if api.dirs[0] != workspaceDir {
		t.Fatalf("dir = %q", api.dirs[0])
	}
}

func TestExecTTYAttachesTerminal(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	api.termOut = "prompt"
	p := newTestProvider(t, api)
	api.add(boxName, statusRunning)
	var stdout bytes.Buffer
	resize := make(chan sandbox.TermSize)
	err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{
		Command: []string{"bash"}, TTY: true, Stdin: strings.NewReader("ls\n"),
		Stdout: &stdout, Size: sandbox.TermSize{Rows: 30, Cols: 90}, Resize: resize,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(api.argvs()) != 0 {
		t.Fatalf("process API ran: %v", api.argvs())
	}
	if len(api.terms) != 1 || api.terms[0].dir != blaxelHome || api.terms[0].rows != 30 || api.terms[0].cols != 90 {
		t.Fatalf("terminal = %+v", api.terms)
	}
	if api.terms[0].stdin != "ls\n" || stdout.String() != "prompt" {
		t.Fatalf("stdin = %q stdout = %q", api.terms[0].stdin, stdout.String())
	}
}

func TestExecTTYSyncedUsesWorkspace(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
	api.add(boxName, statusRunning, sandbox.SyncLabel)
	if err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{TTY: true}); err != nil {
		t.Fatal(err)
	}
	if len(api.terms) != 1 || api.terms[0].dir != workspaceDir {
		t.Fatalf("terminal = %+v", api.terms)
	}
}

func TestExecResizeWithoutTTYIsRejected(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
	api.add(boxName, statusRunning)
	resize := make(chan sandbox.TermSize)
	err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{Command: []string{"sh"}, Resize: resize})
	if err == nil || !strings.Contains(err.Error(), "interactive shell is not available") {
		t.Fatalf("err = %v", err)
	}
	if len(api.argvs()) != 0 || len(api.terms) != 0 {
		t.Fatal("resize without a terminal ran")
	}
}

func TestExecTTYArchivedDoesNotAttach(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
	api.add(boxName, statusArchived)
	err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{TTY: true, Command: []string{"bash"}})
	if err == nil || !strings.Contains(err.Error(), "archived") {
		t.Fatalf("err = %v", err)
	}
	if len(api.terms) != 0 {
		t.Fatal("archived sandbox attached a terminal")
	}
}

func TestExecArchivedSandbox(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
	api.add(boxName, statusArchived)
	err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{Command: []string{"true"}})
	if err == nil || !strings.Contains(err.Error(), "archived") {
		t.Fatalf("err = %v", err)
	}
	if len(api.argvs()) != 0 {
		t.Fatal("archived sandbox ran a command")
	}
}

func TestExecHidesAPIKeyInErrors(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	api.handler = func(runSpec) fakeResult { return fakeResult{err: errors.New("dial failed with test-key")} }
	p := newTestProvider(t, api)
	api.add(boxName, statusRunning)
	err := p.Exec(t.Context(), boxName, sandbox.ExecOptions{Command: []string{"true"}})
	if err == nil || strings.Contains(err.Error(), "test-key") || !strings.Contains(err.Error(), "<redacted>") {
		t.Fatalf("err = %v", err)
	}
}

func TestProcessOutputUsesFinalWhenStreamMisses(t *testing.T) {
	t.Parallel()
	const (
		printf = "a b'$(echo pwned);echo"
		idOut  = "uid\n"
		errOut = "boom\n"
		frame  = "hello"
		line   = "hello\n"
		rest   = "hello\nworld\n"
	)
	tests := []struct {
		name             string
		streamOut        string
		streamErr        string
		finalOut         string
		finalErr         string
		wantOut, wantErr string
	}{
		{name: "fast command", finalOut: idOut, wantOut: idOut},
		{name: "printf without newline", finalOut: printf, wantOut: printf},
		{name: "stderr only", finalErr: errOut, wantErr: errOut},
		{name: "silent command"},
		{name: "stream already showed cat", streamOut: frame, finalOut: line, wantOut: line},
		{name: "stream showed every line", streamOut: line, finalOut: line, wantOut: line},
		{name: "stream showed a prefix", streamOut: line, finalOut: rest, wantOut: rest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got processOutput
			var stdout, stderr bytes.Buffer
			if tt.streamOut != "" {
				got.write(&stdout, &got.stdout, tt.streamOut)
			}
			if tt.streamErr != "" {
				got.write(&stderr, &got.stderr, tt.streamErr)
			}
			got.finish(&stdout, &stderr, tt.finalOut, tt.finalErr)
			if stdout.String() != tt.wantOut || stderr.String() != tt.wantErr {
				t.Fatalf("stdout = %q stderr = %q", stdout.String(), stderr.String())
			}
		})
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
		t.Fatal("character device was piped")
	}
	if pipedInput(strings.NewReader("x")) == nil {
		t.Fatal("reader was dropped")
	}
}
