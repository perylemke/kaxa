package modal_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/provider/modal"
	"github.com/perylemke/kaxa/internal/sandbox"
)

const (
	testWorkspace = "/home/modal/workspace"
	testRunuser   = "runuser"
	testModalUser = "modal"
	testHomeEnv   = "HOME=/home/modal"
	testUserEnv   = "USER=modal"
)

func syncedFake(t *testing.T) *fakeClient {
	t.Helper()
	f := newFakeClient()
	f.boxes[testName] = &modal.Record{ID: testSandboxID, Name: testName, Labels: []string{sandbox.SyncLabel}}
	return f
}

func TestCreatePicksImageByLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		labels    []string
		wantTools bool
	}{
		{"sync label", []string{testKaxa, sandbox.SyncLabel}, true},
		{"no sync label", []string{"kaxa"}, false},
		{"no labels", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFakeClient()
			if _, err := newProvider(t, f).Create(context.Background(), sandbox.Spec{Name: testName, Labels: tt.labels}); err != nil {
				t.Fatalf("create: %v", err)
			}
			if f.withTools != tt.wantTools {
				t.Fatalf("withTools = %v, want %v", f.withTools, tt.wantTools)
			}
		})
	}
}

func TestWorkspaceLayout(t *testing.T) {
	t.Parallel()

	p := modal.New()
	if p.HomeDir() != "/home/modal" {
		t.Fatalf("home = %q", p.HomeDir())
	}
	dir, err := p.RepoDir("api")
	if err != nil || dir != testWorkspace+"/api" {
		t.Fatalf("RepoDir = %q, %v", dir, err)
	}
	for _, bad := range []string{"", ".", "..", "a/b", `a\b`} {
		if _, err := p.RepoDir(bad); err == nil {
			t.Fatalf("RepoDir(%q) succeeded", bad)
		}
	}
}

func TestSyncWorkspaceExtractsAsModal(t *testing.T) {
	t.Parallel()

	f := syncedFake(t)
	err := newProvider(t, f).SyncWorkspace(context.Background(), testName, strings.NewReader("TARBALL"))
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	want := []string{
		testRunuser, "-u", testModalUser, "--", "env", testHomeEnv, testUserEnv, "LOGNAME=modal",
		"tar", "-xzf", "-", "-C", testWorkspace,
	}
	if !slices.Equal(f.execArgv, want) {
		t.Fatalf("argv = %q, want %q", f.execArgv, want)
	}
	if got := f.process.stdin.String(); got != "TARBALL" {
		t.Fatalf("stdin = %q", got)
	}
}

func TestSyncWorkspaceRejectsBadInput(t *testing.T) {
	t.Parallel()

	f := syncedFake(t)
	p := newProvider(t, f)
	if err := p.SyncWorkspace(context.Background(), "Bad Name", strings.NewReader("x")); err == nil {
		t.Fatal("invalid name accepted")
	}
	if err := p.SyncWorkspace(context.Background(), testName, nil); err == nil {
		t.Fatal("nil archive accepted")
	}
	if f.execArgv != nil {
		t.Fatalf("command was started: %v", f.execArgv)
	}
}

func TestSyncWorkspaceReportsStderr(t *testing.T) {
	t.Parallel()

	f := syncedFake(t)
	f.process = newFakeProcess("", "tar: short read\n", 2)

	err := newProvider(t, f).SyncWorkspace(context.Background(), testName, strings.NewReader("x"))
	if err == nil || !strings.Contains(err.Error(), "exit status 2") || !strings.Contains(err.Error(), "tar: short read") {
		t.Fatalf("err = %v, want the exit status and stderr", err)
	}
	// The CLI turns an ExitError into a silent exit code. A failed copy or
	// install is a kaxa error, so it must not be one.
	if _, ok := errors.AsType[*sandbox.ExitError](err); ok {
		t.Fatalf("err = %v wraps *sandbox.ExitError, the CLI would exit without a message", err)
	}
}

func TestSetupProjectsFailureIsNotAnExitError(t *testing.T) {
	t.Parallel()

	f := syncedFake(t)
	f.process = newFakeProcess("", "", 101)

	err := newProvider(t, f).SetupProjects(context.Background(), testName, "api", []project.Step{{
		Kind:     project.KindRust,
		Commands: [][]string{{"cargo", "build"}},
	}})
	if err == nil {
		t.Fatal("setup succeeded although every command failed")
	}
	if _, ok := errors.AsType[*sandbox.ExitError](err); ok {
		t.Fatalf("err = %v wraps *sandbox.ExitError, the CLI would exit without a message", err)
	}
	if !strings.Contains(err.Error(), "exit status 101") {
		t.Fatalf("err = %v, want the exit status", err)
	}
}

func TestSetupProjectsRunsAsModalInRepo(t *testing.T) {
	t.Parallel()

	f := syncedFake(t)
	err := newProvider(t, f).SetupProjects(context.Background(), testName, "api", []project.Step{{
		Kind:     project.KindGo,
		Commands: [][]string{{"go", "build", "./..."}},
	}})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if len(f.execs) == 0 {
		t.Fatal("nothing ran")
	}
	for _, call := range f.execs {
		if len(call.argv) < 4 || call.argv[0] != testRunuser || call.argv[2] != "modal" {
			t.Fatalf("command not run as modal: %q", call.argv)
		}
	}
	build := f.execs[len(f.execs)-1]
	if build.dir != testWorkspace+"/api" {
		t.Fatalf("build dir = %q", build.dir)
	}
	line := strings.Join(build.argv, " ")
	for _, want := range []string{testHomeEnv, testUserEnv, "go build ./..."} {
		if !strings.Contains(line, want) {
			t.Fatalf("build command %q missing %q", line, want)
		}
	}
}

func TestSetupProjectsRejectsBadRepo(t *testing.T) {
	t.Parallel()

	f := syncedFake(t)
	err := newProvider(t, f).SetupProjects(context.Background(), testName, "..", []project.Step{{Kind: project.KindGo}})
	if err == nil || f.execArgv != nil {
		t.Fatalf("err = %v, argv = %v", err, f.execArgv)
	}
}

func TestSetupProjectsWithoutStepsDoesNothing(t *testing.T) {
	t.Parallel()

	f := syncedFake(t)
	if err := newProvider(t, f).SetupProjects(context.Background(), testName, "api", nil); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if f.execArgv != nil {
		t.Fatalf("command was started: %v", f.execArgv)
	}
}

func TestExecOnSyncedSandboxRunsAsModalInWorkspace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		opts     sandbox.ExecOptions
		wantArgv []string
	}{
		{
			name: "command",
			opts: sandbox.ExecOptions{Command: []string{"ls", testLongList}},
			wantArgv: []string{
				testRunuser, "-u", testModalUser, "--", "env", testHomeEnv, testUserEnv, "LOGNAME=modal", "ls", testLongList,
			},
		},
		{
			name: "tty applies stty before switching user",
			opts: sandbox.ExecOptions{Command: []string{testShell}, TTY: true, Size: sandbox.TermSize{Rows: 30, Cols: 100}},
			wantArgv: []string{
				"sh", "-c", `stty rows "$1" cols "$2" 2>/dev/null; shift 2; exec "$@"`, testKaxa, "30", "100",
				testRunuser, "-u", testModalUser, "--", "env", testHomeEnv, testUserEnv, "LOGNAME=modal", testShell,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := syncedFake(t)
			if err := newProvider(t, f).Exec(context.Background(), testName, tt.opts); err != nil {
				t.Fatalf("exec: %v", err)
			}
			if !slices.Equal(f.execArgv, tt.wantArgv) {
				t.Fatalf("argv = %q, want %q", f.execArgv, tt.wantArgv)
			}
			if f.execDir != testWorkspace {
				t.Fatalf("dir = %q, want %q", f.execDir, testWorkspace)
			}
		})
	}
}

func TestExecOnPlainSandboxKeepsRootAndDefaultDir(t *testing.T) {
	t.Parallel()

	f := runningFake(t)
	var out bytes.Buffer
	if err := newProvider(t, f).Exec(context.Background(), testName, sandbox.ExecOptions{Command: []string{"id"}, Stdout: &out}); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !slices.Equal(f.execArgv, []string{"id"}) || f.execDir != "" {
		t.Fatalf("argv = %q dir = %q", f.execArgv, f.execDir)
	}
}
