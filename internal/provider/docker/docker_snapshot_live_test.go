//go:build live

package docker

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// TestRealDockerSnapshot saves a sandbox, restores it into a new one, checks
// that the file written before the snapshot is there and that the new sandbox
// kept the labels kaxa needs, and deletes everything. It bills compute and
// stores two sandboxes and a snapshot for a few minutes.
func TestRealDockerSnapshot(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	src, err := p.Create(ctx, sandbox.Spec{Name: liveName(t), Labels: []string{labelKaxa, sandbox.SyncLabel}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	target := sandbox.ID(liveName(t))
	var snap sandbox.Snapshot
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Minute)
		defer stop()
		_ = p.Destroy(cleanup, src)
		_ = p.Destroy(cleanup, target)
		if snap.ID != "" {
			_ = p.DeleteSnapshot(cleanup, src, snap.ID)
		}
	})

	err = p.Exec(ctx, src, sandbox.ExecOptions{Command: []string{"sh", "-c", "echo kaxa-marker > /home/agent/marker.txt"}})
	if err != nil {
		t.Fatalf("write marker: %v", err)
	}
	snap, err = p.Snapshot(ctx, src, "kaxa live test")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	listed, err := p.ListSnapshots(ctx, src)
	if err != nil || !slices.ContainsFunc(listed, func(s sandbox.Snapshot) bool { return s.ID == snap.ID && s.Comment == "kaxa live test" }) {
		t.Fatalf("list = %+v err = %v, want the snapshot with its comment", listed, err)
	}

	if _, err := p.Restore(ctx, sandbox.RestoreRequest{Sandbox: src, Snapshot: snap.ID, Target: target}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	var out strings.Builder
	err = p.Exec(ctx, target, sandbox.ExecOptions{Command: []string{"cat", "/home/agent/marker.txt"}, Stdout: &out})
	if err != nil || strings.TrimSpace(out.String()) != "kaxa-marker" {
		t.Fatalf("marker in the restored sandbox = %q err = %v", out.String(), err)
	}
	detail, err := p.Inspect(ctx, target)
	if err != nil || !slices.Contains(detail.Labels, sandbox.SyncLabel) {
		t.Fatalf("restored sandbox = %+v err = %v, want the labels of the source", detail, err)
	}

	// Docker refuses to delete a snapshot while a sandbox restored from it runs.
	if err := p.Destroy(ctx, target); err != nil {
		t.Fatalf("destroy the restored sandbox: %v", err)
	}
	if err := p.DeleteSnapshot(ctx, src, snap.ID); err != nil {
		t.Fatalf("delete snapshot: %v", err)
	}
	snap = sandbox.Snapshot{}
}
