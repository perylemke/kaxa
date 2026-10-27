package mcp

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// syncFake is a fakeProvider that can also report an existing repository
// directory. fakeProvider alone has no RepoExists, so it models a provider that
// cannot sync into an existing sandbox.
type syncFake struct {
	*fakeProvider
	exists bool
}

func (s *syncFake) RepoExists(context.Context, sandbox.ID, string) (bool, error) {
	return s.exists, nil
}

// newSyncFake returns a provider whose sandbox carries labels.
func newSyncFake(labels ...string) *syncFake {
	return &syncFake{fakeProvider: &fakeProvider{
		name:   testSprites,
		detail: sandbox.Detail{ID: testBox, Labels: labels},
	}}
}

func withSyncLabel() []string { return []string{"kaxa", sandbox.SyncLabel} }

func initRepoAt(t *testing.T, dir string) {
	t.Helper()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
}

func TestSyncRepositoryCopiesIntoExistingSandbox(t *testing.T) {
	t.Parallel()

	repo := gitRepo(t)
	fp := newSyncFake(withSyncLabel()...)
	svc := newTestService(t, repo, testSprites, fp)

	out, err := svc.syncRepository(context.Background(), syncIn{Name: testBox})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	want := syncOut{ID: testBox, Provider: testSprites, WorkDir: "/home/box/ws/" + filepath.Base(repo)}
	if out != want {
		t.Fatalf("out = %+v, want %+v", out, want)
	}
	if !fp.synced || len(fp.created) != 0 || len(fp.destroyed) != 0 {
		t.Fatalf("synced=%v created=%v destroyed=%v", fp.synced, fp.created, fp.destroyed)
	}
}

func TestSyncRepositoryDirInsideServerRoot(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	repo := filepath.Join(parent, "app")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	initRepoAt(t, repo)
	fp := newSyncFake(withSyncLabel()...)
	svc := newTestService(t, parent, testSprites, fp)

	out, err := svc.syncRepository(context.Background(), syncIn{Name: testBox, Dir: repo})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if out.WorkDir != "/home/box/ws/app" {
		t.Fatalf("work dir = %q", out.WorkDir)
	}
}

func TestSyncRepositoryRejectsRepoOutsideServerRoot(t *testing.T) {
	t.Parallel()

	fp := newSyncFake(withSyncLabel()...)
	svc := newTestService(t, gitRepo(t), testSprites, fp)

	_, err := svc.syncRepository(context.Background(), syncIn{Name: testBox, Dir: gitRepo(t)})
	wantErr(t, err, "outside the server working directory")
	if fp.synced {
		t.Fatal("an unrelated repository was copied")
	}
}

func TestSyncRepositoryFailsOutsideGitRepo(t *testing.T) {
	t.Parallel()

	fp := newSyncFake(withSyncLabel()...)
	svc := newTestService(t, t.TempDir(), testSprites, fp)

	_, err := svc.syncRepository(context.Background(), syncIn{Name: testBox})
	wantErr(t, err, "not inside a git repository")
	if fp.synced {
		t.Fatal("repository was copied")
	}
}

func TestSyncRepositoryRejectsSandboxWithoutWorkspace(t *testing.T) {
	t.Parallel()

	fp := newSyncFake("kaxa")
	svc := newTestService(t, gitRepo(t), testSprites, fp)

	_, err := svc.syncRepository(context.Background(), syncIn{Name: testBox})
	wantErr(t, err, "was not created with sync")
	if fp.synced {
		t.Fatal("repository was copied")
	}
}

func TestSyncRepositoryRefusesExistingRepository(t *testing.T) {
	t.Parallel()

	fp := newSyncFake(withSyncLabel()...)
	fp.exists = true
	svc := newTestService(t, gitRepo(t), testSprites, fp)

	_, err := svc.syncRepository(context.Background(), syncIn{Name: testBox})
	wantErr(t, err, "already exists")
	if fp.synced {
		t.Fatal("repository was copied over an existing directory")
	}
}

func TestSyncRepositoryFailureKeepsSandbox(t *testing.T) {
	t.Parallel()

	fp := newSyncFake(withSyncLabel()...)
	fp.syncErr = errors.New("copy failed")
	svc := newTestService(t, gitRepo(t), testSprites, fp)

	_, err := svc.syncRepository(context.Background(), syncIn{Name: testBox})
	wantErr(t, err, "copy failed")
	if len(fp.destroyed) != 0 {
		t.Fatalf("sandbox was destroyed: %v", fp.destroyed)
	}
}

func TestSyncRepositoryRejectsProviderWithoutSupport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		p    sandbox.Provider
		want string
	}{
		{"no sync at all", plainProvider{name: testPlain}, "sync is not available for plain"},
		{"cannot check the directory", &fakeProvider{name: testSprites, detail: sandbox.Detail{ID: testBox, Labels: withSyncLabel()}}, "sync is not available for sprites"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := newTestService(t, gitRepo(t), tt.p.Name(), tt.p)

			_, err := svc.syncRepository(context.Background(), syncIn{Name: testBox})
			wantErr(t, err, tt.want)
		})
	}
}

func TestSyncRepositoryRequiresName(t *testing.T) {
	t.Parallel()

	fp := newSyncFake(withSyncLabel()...)
	svc := newTestService(t, gitRepo(t), testSprites, fp)

	_, err := svc.syncRepository(context.Background(), syncIn{})
	wantErr(t, err, "name is required")
	if fp.synced {
		t.Fatal("repository was copied")
	}
}
