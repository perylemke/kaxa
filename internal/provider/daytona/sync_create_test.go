package daytona

import (
	"os"
	"os/exec"
	"slices"
	"testing"

	"github.com/perylemke/kaxa/internal/lifecycle"
)

// TestCreateSyncWhileNameIsUnreadable is the failure where Find misses a sandbox
// it just created, because the labels are not visible yet. Creating the
// workspace uses the handle from Create, so the copy still happens.
func TestCreateSyncWhileNameIsUnreadable(t *testing.T) {
	tb := newFakeToolbox(okHandler)
	p, api := newTestProvider(t, tb)
	api.mu.Lock()
	api.blindFind = true
	api.mu.Unlock()
	tb.afterRun = func() {
		api.mu.Lock()
		api.blindFind = false
		api.mu.Unlock()
	}

	dir := t.TempDir()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	root, repo, err := lifecycle.ResolveRepo(dir, true)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	res, err := lifecycle.Create(t.Context(), p, lifecycle.Request{
		Name: boxName, Sync: true, Root: root, Repo: repo,
	}, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(api.sandboxes) == 0 {
		t.Fatal("sandbox was destroyed")
	}
	got := tb.argvs()
	if len(got) == 0 || !slices.Equal(got[0], []string{"mkdir", "-p", workspaceDir}) {
		t.Fatalf("commands = %v", got)
	}
	if res.WorkDir != workspaceDir+"/"+repo {
		t.Fatalf("work dir = %q", res.WorkDir)
	}
}
