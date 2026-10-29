//go:build live

package sprites_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/provider/sprites"
	"github.com/perylemke/kaxa/internal/sandbox"
)

// TestRealSpritesSnapshotRoundTrip snapshots a sprite, changes it, restores the
// snapshot in place, and checks the old content is back. It also exercises the
// checkpoint delete route, which the public API reference does not document.
// It is skipped without KAXA_SPRITES_TOKEN.
func TestRealSpritesSnapshotRoundTrip(t *testing.T) {
	token := os.Getenv("KAXA_SPRITES_TOKEN")
	if token == "" {
		t.Skip("KAXA_SPRITES_TOKEN is not set")
	}
	p := sprites.New()
	if err := p.Configure(token); err != nil {
		t.Fatalf("configure: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	id, err := p.Create(ctx, sandbox.Spec{Name: "kaxa-it-snap-" + hex.EncodeToString(b), Labels: []string{"kaxa", "integration"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		_ = p.Destroy(cleanup, id)
	})

	run := func(script string) string {
		t.Helper()
		var out strings.Builder
		if err := p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{"sh", "-c", script}, Stdout: &out}); err != nil {
			t.Fatalf("exec %q: %v", script, err)
		}
		return strings.TrimSpace(out.String())
	}

	// /tmp is tmpfs and is not part of a checkpoint; /home/sprite is.
	const stateFile = "/home/sprite/state.txt"

	run("echo original > " + stateFile)
	original, err := p.Snapshot(ctx, id, "integration original")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	run("echo changed > " + stateFile)
	changed, err := p.Snapshot(ctx, id, "integration changed")
	if err != nil {
		t.Fatalf("second snapshot: %v", err)
	}

	list, err := p.ListSnapshots(ctx, id)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !hasSnapshot(list, original.ID) || !hasSnapshot(list, changed.ID) {
		t.Fatalf("list %+v does not contain %q and %q", list, original.ID, changed.ID)
	}

	if got, err := p.Restore(ctx, sandbox.RestoreRequest{Sandbox: id, Snapshot: original.ID}); err != nil || got != id {
		t.Fatalf("restore = %q, %v", got, err)
	}
	// Restore can finish in the background, so wait for the sprite to come back.
	waitForContent(t, ctx, p, id, stateFile, "original")

	if err := p.DeleteSnapshot(ctx, id, changed.ID); err != nil {
		t.Fatalf("delete snapshot: %v", err)
	}
	list, err = p.ListSnapshots(ctx, id)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if hasSnapshot(list, changed.ID) {
		t.Fatalf("snapshot %q still listed after delete", changed.ID)
	}
	if !hasSnapshot(list, original.ID) {
		t.Fatalf("snapshot %q disappeared", original.ID)
	}
	if err := p.DeleteSnapshot(ctx, id, original.ID); err != nil {
		t.Fatalf("delete restored snapshot: %v", err)
	}
}

func hasSnapshot(list []sandbox.Snapshot, id sandbox.SnapshotID) bool {
	for _, s := range list {
		if s.ID == id {
			return true
		}
	}
	return false
}

// waitForContent reads path until it holds want, and logs how long that took.
// A read can fail while the sprite restarts, so errors are retried.
func waitForContent(t *testing.T, ctx context.Context, p *sprites.Provider, id sandbox.ID, path, want string) {
	t.Helper()
	start := time.Now()
	deadline := start.Add(90 * time.Second)
	var last string
	for {
		var out strings.Builder
		err := p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{"cat", path}, Stdout: &out})
		last = strings.TrimSpace(out.String())
		if err == nil && last == want {
			t.Logf("%s held %q %s after restore returned", path, want, time.Since(start).Round(time.Millisecond))
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s = %q (exec error %v), want %q after 90s", path, last, err, want)
		}
		time.Sleep(2 * time.Second)
	}
}
