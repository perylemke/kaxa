package sprites_test

import (
	"testing"

	"github.com/perylemke/kaxa/internal/provider/sprites"
	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestWorkspaceLayout(t *testing.T) {
	t.Parallel()

	p := sprites.New()
	var layout sandbox.WorkspaceLayout = p
	if got := layout.HomeDir(); got != "/home/sprite" {
		t.Fatalf("HomeDir = %q", got)
	}
	got, err := layout.RepoDir("api-python")
	if err != nil {
		t.Fatalf("RepoDir: %v", err)
	}
	if got != "/home/sprite/workspace/api-python" {
		t.Fatalf("RepoDir = %q", got)
	}
}

func TestWorkspaceLayoutRejectsBadSegment(t *testing.T) {
	t.Parallel()

	p := sprites.New()
	for _, repo := range []string{"", ".", "..", "a/b", `a\b`, "a\x00b"} {
		if _, err := p.RepoDir(repo); err == nil {
			t.Errorf("RepoDir(%q) accepted", repo)
		}
	}
}

func TestAvailable(t *testing.T) {
	t.Parallel()

	if !sandbox.IsAvailable(sprites.New()) {
		t.Fatal("sprites should be available")
	}
}
