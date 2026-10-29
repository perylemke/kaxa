//go:build live

package modal_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/provider/modal"
	"github.com/perylemke/kaxa/internal/sandbox"
)

func realModal(t *testing.T) *modal.Provider {
	t.Helper()
	id, secret := os.Getenv("KAXA_MODAL_TOKEN_ID"), os.Getenv("KAXA_MODAL_TOKEN_SECRET")
	if id == "" || secret == "" {
		t.Skip("KAXA_MODAL_TOKEN_ID and KAXA_MODAL_TOKEN_SECRET are not set")
	}
	p := modal.New()
	if err := p.ConfigureCredentials(id, secret); err != nil {
		t.Fatalf("configure: %v", err)
	}
	p.SetSnapshotIndexPath(filepath.Join(t.TempDir(), "snapshots.json"))
	return p
}

func execOutput(t *testing.T, ctx context.Context, p sandbox.Provider, id sandbox.ID, script string) string {
	t.Helper()
	var out strings.Builder
	if err := p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{"sh", "-c", script}, Stdout: &out}); err != nil {
		t.Fatalf("exec %q on %s: %v", script, id, err)
	}
	return strings.TrimSpace(out.String())
}

func destroyLater(t *testing.T, p sandbox.Provider, id sandbox.ID) {
	t.Helper()
	t.Cleanup(func() {
		ctx, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		_ = p.Destroy(ctx, id)
	})
}

// TestRealModalSnapshotRoundTrip snapshots a sandbox, changes it, restores the
// snapshot into a new sandbox, and checks the new one has the old content and
// the source kept the change. It bills a few minutes of compute.
func TestRealModalSnapshotRoundTrip(t *testing.T) {
	p := realModal(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	src := sandbox.ID("kaxa-it-snap-" + randomSuffix(t))
	if _, err := p.Create(ctx, sandbox.Spec{Name: string(src), Labels: []string{"kaxa", "integration"}}); err != nil {
		t.Fatalf("create: %v", err)
	}
	destroyLater(t, p, src)

	execOutput(t, ctx, p, src, "echo original > /tmp/state.txt")
	snap, err := p.Snapshot(ctx, src, "integration")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		_ = p.DeleteSnapshot(cleanup, src, snap.ID)
	})

	execOutput(t, ctx, p, src, "echo changed > /tmp/state.txt")

	list, err := p.ListSnapshots(ctx, src)
	if err != nil || len(list) != 1 || list[0].ID != snap.ID {
		t.Fatalf("list = %+v, err = %v", list, err)
	}

	dst := sandbox.ID(string(src) + "-copy")
	restored, err := p.Restore(ctx, sandbox.RestoreRequest{Sandbox: src, Snapshot: snap.ID, Target: dst})
	if err != nil || restored != dst {
		t.Fatalf("restore = %q, %v", restored, err)
	}
	destroyLater(t, p, dst)

	if got := execOutput(t, ctx, p, dst, "cat /tmp/state.txt"); got != "original" {
		t.Fatalf("restored content = %q, want original", got)
	}
	if got := execOutput(t, ctx, p, src, "cat /tmp/state.txt"); got != "changed" {
		t.Fatalf("source content = %q, want changed", got)
	}

	if err := p.DeleteSnapshot(ctx, src, snap.ID); err != nil {
		t.Fatalf("delete snapshot: %v", err)
	}
	if list, _ := p.ListSnapshots(ctx, src); len(list) != 0 {
		t.Fatalf("list after delete = %+v", list)
	}
}

// TestRealModalSnapshotKeepsSyncUser checks that a sandbox restored from a
// --sync sandbox still runs as the modal user in /home/modal/workspace. It
// builds the sync image on first use and can take several minutes.
func TestRealModalSnapshotKeepsSyncUser(t *testing.T) {
	p := realModal(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	src := sandbox.ID("kaxa-it-snapsync-" + randomSuffix(t))
	if _, err := p.Create(ctx, sandbox.Spec{Name: string(src), Labels: []string{"kaxa", sandbox.SyncLabel}}); err != nil {
		t.Fatalf("create: %v", err)
	}
	destroyLater(t, p, src)
	if err := p.SyncWorkspace(ctx, src, tinyRepoArchive(t)); err != nil {
		t.Fatalf("sync: %v", err)
	}

	snap, err := p.Snapshot(ctx, src, "")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		_ = p.DeleteSnapshot(cleanup, src, snap.ID)
	})

	dst := sandbox.ID(string(src) + "-copy")
	if _, err := p.Restore(ctx, sandbox.RestoreRequest{Sandbox: src, Snapshot: snap.ID, Target: dst}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	destroyLater(t, p, dst)

	got := strings.Fields(execOutput(t, ctx, p, dst, "pwd; id -un; cat demo/hello.txt"))
	if len(got) != 3 || got[0] != "/home/modal/workspace" || got[1] != "modal" || got[2] != "hi" {
		t.Fatalf("restored output = %q, want workspace, modal, hi", got)
	}
}
