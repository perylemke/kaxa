package cli_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/cli"
	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	testCmdSync  = "sync"
	testHelpFlag = "--help"
)

// repoSyncProvider is a syncingProvider whose sandbox may already hold the repository.
type repoSyncProvider struct {
	*syncingProvider
	exists bool
}

func (r *repoSyncProvider) RepoExists(context.Context, sandbox.ID, string) (bool, error) {
	return r.exists, nil
}

func (r *repoSyncProvider) HomeDir() string { return "/home/box" }
func (r *repoSyncProvider) RepoDir(repo string) (string, error) {
	return "/home/box/ws/" + repo, nil
}

func newRepoSyncProvider(labels ...string) *repoSyncProvider {
	return &repoSyncProvider{syncingProvider: &syncingProvider{
		fakeProvider: &fakeProvider{
			name:   testSpritesName,
			detail: sandbox.Detail{ID: testBoxName, Labels: labels},
		},
	}}
}

func runSync(t *testing.T, p sandbox.Provider, args ...string) (*cobra.Command, *bytes.Buffer, error) {
	t.Helper()
	cmd := newCreateCmd(t, p)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetArgs(append([]string{testProviderFlag, testSpritesName, testCmdSync}, args...))
	return cmd, out, cmd.Execute()
}

func TestSyncCopiesRepositoryAndInstalls(t *testing.T) {
	repo := initGitRepo(t)
	writeGoMod(t)

	fp := newRepoSyncProvider("kaxa", sandbox.SyncLabel)
	_, out, err := runSync(t, fp, testBoxName)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !fp.synced || fp.setupRepo != filepath.Base(repo) {
		t.Fatalf("synced=%v setup repo=%q", fp.synced, fp.setupRepo)
	}
	if len(fp.setupSteps) != 1 || fp.setupSteps[0].Kind != project.KindGo {
		t.Fatalf("steps = %#v", fp.setupSteps)
	}
	want := "/home/box/ws/" + filepath.Base(repo)
	if !strings.Contains(out.String(), want) || !strings.Contains(out.String(), testBoxName) {
		t.Fatalf("output = %q, want the sandbox and %q", out.String(), want)
	}
	if fp.destroyed != "" {
		t.Fatalf("destroyed = %q", fp.destroyed)
	}
}

func TestSyncDirCopiesNamedRepository(t *testing.T) {
	repo := initGitRepo(t)
	t.Chdir(t.TempDir())

	fp := newRepoSyncProvider("kaxa", sandbox.SyncLabel)
	if _, _, err := runSync(t, fp, testBoxName, testDirFlag, repo); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !fp.synced {
		t.Fatal("repository was not copied")
	}
}

func TestSyncRequiresOneSandboxName(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"missing", nil, "sync requires a sandbox name"},
		{"two names", []string{testBoxName, "other"}, "sync accepts one sandbox name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initGitRepo(t)
			fp := newRepoSyncProvider("kaxa", sandbox.SyncLabel)

			_, _, err := runSync(t, fp, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
			if fp.synced {
				t.Fatal("repository was copied")
			}
		})
	}
}

func TestSyncRejectsSandboxWithoutWorkspace(t *testing.T) {
	initGitRepo(t)

	fp := newRepoSyncProvider("kaxa")
	_, _, err := runSync(t, fp, testBoxName)
	if err == nil || !strings.Contains(err.Error(), "was not created with --sync") {
		t.Fatalf("err = %v", err)
	}
	if fp.synced {
		t.Fatal("repository was copied")
	}
}

func TestSyncRefusesExistingRepository(t *testing.T) {
	initGitRepo(t)

	fp := newRepoSyncProvider("kaxa", sandbox.SyncLabel)
	fp.exists = true
	_, _, err := runSync(t, fp, testBoxName)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v", err)
	}
	if fp.synced {
		t.Fatal("repository was copied over an existing directory")
	}
}

func TestSyncFailureKeepsSandbox(t *testing.T) {
	initGitRepo(t)
	writeGoMod(t)

	tests := []struct {
		name  string
		setup func(*repoSyncProvider)
		want  string
	}{
		{"copy fails", func(p *repoSyncProvider) { p.syncErr = errors.New("copy failed") }, "copy failed"},
		{"install fails", func(p *repoSyncProvider) { p.setupErr = errors.New("go build failed") }, "go build failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := newRepoSyncProvider("kaxa", sandbox.SyncLabel)
			tt.setup(fp)

			_, _, err := runSync(t, fp, testBoxName)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
			if fp.destroyed != "" {
				t.Fatalf("sandbox was destroyed: %q", fp.destroyed)
			}
		})
	}
}

func TestSyncRejectsBadRepositoryBeforeTouchingSandbox(t *testing.T) {
	t.Chdir(t.TempDir())
	plain := t.TempDir()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"current directory is not a repository", []string{testBoxName}, "current directory is not inside a git repository"},
		{"missing --dir", []string{testBoxName, testDirFlag, filepath.Join(plain, "missing")}, "was not found"},
		{"--dir is not a repository", []string{testBoxName, testDirFlag, plain}, "is not inside a git repository"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := newRepoSyncProvider("kaxa", sandbox.SyncLabel)

			_, _, err := runSync(t, fp, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
			if fp.synced {
				t.Fatal("repository was copied")
			}
		})
	}
}

func TestSyncHintNamesTheSyncCommand(t *testing.T) {
	t.Chdir(t.TempDir())

	fp := newRepoSyncProvider("kaxa", sandbox.SyncLabel)
	_, _, err := runSync(t, fp, testBoxName)
	ue, ok := usererr.As(err)
	if !ok || !strings.Contains(ue.Hint, "kaxa sync") || strings.Contains(ue.Hint, "kaxa create") {
		t.Fatalf("err = %v, hint = %+v", err, ue)
	}
}

func TestSyncRejectsProviderWithoutSync(t *testing.T) {
	initGitRepo(t)

	fp := &fakeProvider{name: testSpritesName}
	_, _, err := runSync(t, fp, testBoxName)
	if err == nil || !strings.Contains(err.Error(), "sync is not available for") {
		t.Fatalf("err = %v", err)
	}
}

func TestRootHelpListsSync(t *testing.T) {
	t.Parallel()

	cmd := cli.NewRootCmd(testRegistry(t))
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetArgs([]string{testHelpFlag})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), testCmdSync) {
		t.Fatalf("help should list sync: %s", out.String())
	}
}
