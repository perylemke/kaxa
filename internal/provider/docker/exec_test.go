package docker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/sandbox"
)

func output(s string) Event    { return Event{Data: []byte(s)} }
func errOutput(s string) Event { return Event{Data: []byte(s), Stderr: true} }
func exited(code int) Event    { return Event{Exited: true, ExitCode: code} }

func TestCommandOf(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		argv []string
		env  []string
		want []string
	}{
		{"plain", []string{cmdEcho, "hi"}, nil, []string{cmdEcho, "hi"}},
		{cmdEnv, []string{"go", "version"}, []string{"A=1", "B=2"}, []string{cmdEnv, "A=1", "B=2", "go", "version"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := commandOf(tt.argv, tt.env); !slices.Equal(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExecStreamsOutputAndStdin(t *testing.T) {
	t.Parallel()
	p, api, proc := newTestProvider(t)
	createBox(t, p)
	proc.script = func(_ ProcessSpec, stdin string) []Event {
		return []Event{output("out:" + stdin), errOutput("warn")}
	}
	var stdout, stderr bytes.Buffer
	err := p.Exec(context.Background(), testSandbox, sandbox.ExecOptions{
		Command: []string{cmdCat}, Stdin: strings.NewReader("data"), Stdout: &stdout, Stderr: &stderr,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "out:data" || stderr.String() != "warn" {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
	spec := proc.lastSpec(t)
	if !slices.Equal(spec.Argv, []string{cmdCat}) || spec.User != agentUser || spec.TTY {
		t.Fatalf("spec = %+v", spec)
	}
	if proc.tokens[0] != testToken || proc.eps[0].URI == "" {
		t.Fatalf("credential %q, endpoint %q", proc.tokens[0], proc.eps[0].URI)
	}
	if api.renewed == 0 {
		t.Fatal("the time to live must be renewed")
	}
}

func TestExecExitCode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		code int
		want int
	}{
		{"failure", 3, 3},
		{"signal", -9, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, _, proc := newTestProvider(t)
			createBox(t, p)
			proc.script = func(ProcessSpec, string) []Event { return []Event{exited(tt.code)} }
			err := p.Exec(context.Background(), testSandbox, sandbox.ExecOptions{Command: []string{"false"}})
			exitErr, ok := errors.AsType[*sandbox.ExitError](err)
			if !ok || exitErr.Code != tt.want {
				t.Fatalf("err = %v, want exit code %d", err, tt.want)
			}
		})
	}
}

func TestExecRejectsEmptyCommand(t *testing.T) {
	t.Parallel()
	p, _, _ := newTestProvider(t)
	createBox(t, p)
	if err := p.Exec(context.Background(), testSandbox, sandbox.ExecOptions{}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestExecUnknownSandbox(t *testing.T) {
	t.Parallel()
	p, _, proc := newTestProvider(t)
	err := p.Exec(context.Background(), "nope", sandbox.ExecOptions{Command: []string{"ls"}})
	if !errors.Is(err, ErrNotFound) || len(proc.specs) != 0 {
		t.Fatalf("err = %v, specs = %d", err, len(proc.specs))
	}
}

func TestExecInSyncedSandbox(t *testing.T) {
	t.Parallel()
	p, _, proc := newTestProvider(t)
	createBox(t, p, labelKaxa, sandbox.SyncLabel)
	if err := p.Exec(context.Background(), testSandbox, sandbox.ExecOptions{Command: []string{"go", "test"}}); err != nil {
		t.Fatal(err)
	}
	spec := proc.lastSpec(t)
	if spec.Dir != "/home/agent/workspace" || spec.Argv[0] != "env" || !slices.Contains(spec.Argv, "HOME=/home/agent") {
		t.Fatalf("spec = %+v", spec)
	}
	if !slices.Equal(spec.Argv[len(spec.Argv)-2:], []string{"go", "test"}) {
		t.Fatalf("argv = %v", spec.Argv)
	}
}

func TestExecTTYPassesSizeAndResizes(t *testing.T) {
	t.Parallel()
	p, _, proc := newTestProvider(t)
	createBox(t, p)
	resize := make(chan sandbox.TermSize, 1)
	resize <- sandbox.TermSize{Rows: 40, Cols: 100}
	proc.script = func(ProcessSpec, string) []Event { return []Event{output("$ ")} }
	var stdout bytes.Buffer
	err := p.Exec(context.Background(), testSandbox, sandbox.ExecOptions{
		Command: []string{"bash"}, TTY: true, Size: sandbox.TermSize{Rows: 30, Cols: 90}, Resize: resize,
		Stdin: strings.NewReader(""), Stdout: &stdout,
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := proc.lastSpec(t)
	if !spec.TTY || spec.Rows != 30 || spec.Cols != 90 || spec.Env["TERM"] != termName {
		t.Fatalf("spec = %+v", spec)
	}
	if stdout.String() != "$ " {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestExecStartsStoppedSandbox(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	createBox(t, p)
	api.setStatus(testSandbox, statusStopped)
	if err := p.Exec(context.Background(), testSandbox, sandbox.ExecOptions{Command: []string{"ls"}}); err != nil {
		t.Fatal(err)
	}
	if api.seen("start") != 1 {
		t.Fatalf("start calls = %d, want 1", api.seen("start"))
	}
}

func TestExecWaitsForStartingSandbox(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	createBox(t, p)
	api.setStatus(testSandbox, statusNew)
	api.mu.Lock()
	for res := range api.recs {
		api.next[res] = statusRunning
	}
	api.mu.Unlock()
	if err := p.Exec(context.Background(), testSandbox, sandbox.ExecOptions{Command: []string{"ls"}}); err != nil {
		t.Fatal(err)
	}
	if api.seen("start") != 0 {
		t.Fatal("a sandbox that is starting must not be started again")
	}
}

func TestExecErrorsDoNotLeakCredentials(t *testing.T) {
	t.Parallel()
	p, _, proc := newTestProvider(t)
	createBox(t, p)
	proc.err = errors.New("dial failed with " + testToken + " and " + testPAT)
	err := p.Exec(context.Background(), testSandbox, sandbox.ExecOptions{Command: []string{"ls"}})
	if err == nil || strings.Contains(err.Error(), testToken) || strings.Contains(err.Error(), testPAT) {
		t.Fatalf("err = %v, must hold no credential", err)
	}
}

func TestExecCredentialFailure(t *testing.T) {
	t.Parallel()
	p, api, proc := newTestProvider(t)
	createBox(t, p)
	api.failOn["credential"] = &APIError{Status: 403}
	err := p.Exec(context.Background(), testSandbox, sandbox.ExecOptions{Command: []string{"ls"}})
	if err == nil || len(proc.specs) != 0 {
		t.Fatalf("err = %v, specs = %d", err, len(proc.specs))
	}
}

func TestExecStopsWhenContextIsCanceled(t *testing.T) {
	t.Parallel()
	p, _, proc := newTestProvider(t)
	createBox(t, p)
	proc.script = func(ProcessSpec, string) []Event { return nil }
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	defer func() { _ = pw.Close() }()
	done := make(chan error, 1)
	go func() {
		done <- p.Exec(ctx, testSandbox, sandbox.ExecOptions{Command: []string{cmdCat}, Stdin: pr})
	}()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestSyncWorkspaceStreamsArchive(t *testing.T) {
	t.Parallel()
	p, _, proc := newTestProvider(t)
	createBox(t, p, labelKaxa, sandbox.SyncLabel)
	if err := p.SyncWorkspace(context.Background(), testSandbox, strings.NewReader("TARBALL")); err != nil {
		t.Fatal(err)
	}
	argvs := proc.argvs()
	if len(argvs) != 2 || !strings.Contains(strings.Join(argvs[0], " "), "mkdir -p /home/agent/workspace") {
		t.Fatalf("argvs = %v", argvs)
	}
	tar := strings.Join(argvs[1], " ")
	if !strings.Contains(tar, "tar -xzf - -C /home/agent/workspace") {
		t.Fatalf("tar command = %q", tar)
	}
	if got := proc.stdins[1].String(); got != "TARBALL" {
		t.Fatalf("archive on stdin = %q", got)
	}
}

func TestSyncWorkspaceFailureCarriesStderr(t *testing.T) {
	t.Parallel()
	p, _, proc := newTestProvider(t)
	createBox(t, p)
	proc.script = func(spec ProcessSpec, _ string) []Event {
		if slices.Contains(spec.Argv, "tar") {
			return []Event{errOutput("tar: bad archive"), exited(2)}
		}
		return nil
	}
	err := p.SyncWorkspace(context.Background(), testSandbox, strings.NewReader("x"))
	if err == nil || !strings.Contains(err.Error(), "tar: bad archive") {
		t.Fatalf("err = %v", err)
	}
	if err := p.SyncWorkspace(context.Background(), testSandbox, nil); err == nil {
		t.Fatal("a nil archive must fail")
	}
}

func TestRepoDirAndHomeDir(t *testing.T) {
	t.Parallel()
	p := New()
	if got := p.HomeDir(); got != "/home/agent" {
		t.Fatalf("HomeDir = %q", got)
	}
	dir, err := p.RepoDir("app")
	if err != nil || dir != "/home/agent/workspace/app" {
		t.Fatalf("RepoDir = %q, %v", dir, err)
	}
	for _, bad := range []string{"", ".", "..", "a/b", `a\b`} {
		if _, err := p.RepoDir(bad); err == nil {
			t.Errorf("RepoDir(%q) must fail", bad)
		}
	}
}

func TestRepoExists(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		events  []Event
		want    bool
		wantErr bool
	}{
		{"present", nil, true, false},
		{"absent", []Event{exited(1)}, false, false},
		{"broken", []Event{exited(2)}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, _, proc := newTestProvider(t)
			createBox(t, p)
			proc.script = func(ProcessSpec, string) []Event { return tt.events }
			got, err := p.RepoExists(context.Background(), testSandbox, "app")
			if got != tt.want || (err != nil) != tt.wantErr {
				t.Fatalf("got %v, %v", got, err)
			}
			if spec := proc.lastSpec(t); !slices.Equal(spec.Argv, []string{"test", "-d", "/home/agent/workspace/app"}) {
				t.Fatalf("argv = %v", spec.Argv)
			}
		})
	}
}

func TestSetupProjects(t *testing.T) {
	t.Parallel()
	p, _, proc := newTestProvider(t)
	ctx := context.Background()
	createBox(t, p, labelKaxa, sandbox.SyncLabel)
	steps := []project.Step{{Kind: project.KindGo, Commands: [][]string{{"go", "mod", "download"}}}}
	if err := p.SetupProjects(ctx, testSandbox, "app", steps); err != nil {
		t.Fatal(err)
	}
	spec := proc.lastSpec(t)
	if spec.Dir != "/home/agent/workspace/app" || !slices.Equal(spec.Argv[len(spec.Argv)-3:], []string{"go", "mod", "download"}) {
		t.Fatalf("spec = %+v", spec)
	}
	if !slices.Contains(spec.Argv, "HOME=/home/agent") {
		t.Fatalf("toolchain env missing: %v", spec.Argv)
	}
	before := len(proc.argvs())
	if err := p.SetupProjects(ctx, testSandbox, "app", nil); err != nil || len(proc.argvs()) != before {
		t.Error("no steps must not run anything")
	}
	if err := p.SetupProjects(ctx, testSandbox, "a/b", steps); err == nil {
		t.Error("an invalid repository must fail")
	}
}

func TestRestartStopsThenStarts(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	createBox(t, p)
	if err := p.Restart(context.Background(), testSandbox); err != nil {
		t.Fatal(err)
	}
	stop, start := slices.Index(api.calls, "stop"), slices.Index(api.calls, "start")
	if stop < 0 || start < stop {
		t.Fatalf("calls = %v, want stop before start", api.calls)
	}
	d, err := p.Inspect(context.Background(), testSandbox)
	if err != nil || d.Status != statusRunning {
		t.Fatalf("status = %q, err = %v", d.Status, err)
	}
}

func TestRestartStoppedSandboxOnlyStarts(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	createBox(t, p)
	api.setStatus(testSandbox, statusStopped)
	if err := p.Restart(context.Background(), testSandbox); err != nil {
		t.Fatal(err)
	}
	if api.seen("stop") != 0 || api.seen("start") != 1 {
		t.Fatalf("calls = %v", api.calls)
	}
}

func TestRestartUnknown(t *testing.T) {
	t.Parallel()
	p, _, _ := newTestProvider(t)
	if err := p.Restart(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestCapabilities(t *testing.T) {
	t.Parallel()
	caps := sandbox.CapabilitiesOf(New())
	for feature, want := range map[sandbox.Feature]bool{
		sandbox.FeatureSync:              true,
		sandbox.FeatureDependencyInstall: true,
		sandbox.FeatureRestart:           true,
		sandbox.FeatureTemplate:          true,
		sandbox.FeatureAPIPublish:        true,
		sandbox.FeatureRunning:           false,
		sandbox.FeatureSnapshot:          true,
		sandbox.FeatureUpgrade:           false,
		sandbox.FeaturePublicURLs:        false,
	} {
		if got := caps.Has(feature); got != want {
			t.Errorf("Has(%q) = %v, want %v", feature, got, want)
		}
	}
	if !caps.Layout {
		t.Error("provider must report a workspace layout")
	}
}
