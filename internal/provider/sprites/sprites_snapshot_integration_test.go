//go:build integration

package sprites_test

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/kaxa/internal/provider/sprites"
	"github.com/perylemke/kaxa/internal/sandbox"
)

// checkpointFake is an in-memory Sprites client with checkpoints. Like the
// real API, its list also holds a "Current" entry for the live state.
type checkpointFake struct {
	*fakeSDKClient

	mu        sync.Mutex
	saved     []*spritesdk.Checkpoint
	restored  []string
	deleted   []string
	destroyed []string
}

func newCheckpointFake(token string) *checkpointFake {
	f := &checkpointFake{fakeSDKClient: &fakeSDKClient{}}
	f.create = func(_ context.Context, name string, _ []string) (*spritesdk.Sprite, error) {
		return spritesdk.New(token).Sprite(name), nil
	}
	f.destroy = func(_ context.Context, name string) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.destroyed = append(f.destroyed, name)
		return nil
	}
	return f
}

func (f *checkpointFake) CreateCheckpoint(_ context.Context, _, comment string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := len(f.saved) + 1
	f.saved = append(f.saved, &spritesdk.Checkpoint{
		ID:         fmt.Sprintf("v%d", n),
		CreateTime: time.Date(2026, 1, 1, 0, n, 0, 0, time.UTC),
		Comment:    comment,
	})
	return nil
}

func (f *checkpointFake) ListCheckpoints(context.Context, string) ([]*spritesdk.Checkpoint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	live := &spritesdk.Checkpoint{ID: "Current", CreateTime: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}
	return append([]*spritesdk.Checkpoint{live}, f.saved...), nil
}

func (f *checkpointFake) RestoreCheckpoint(_ context.Context, _, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restored = append(f.restored, id)
	return nil
}

func (f *checkpointFake) DeleteCheckpoint(_ context.Context, _, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, id)
	f.saved = slices.DeleteFunc(f.saved, func(cp *spritesdk.Checkpoint) bool { return cp.ID == id })
	return nil
}

// TestSpritesSnapshotRoundTrip follows the flow of TestRealSpritesSnapshotRoundTrip
// (sprites_snapshot_live_test.go) against the in-memory client, so it costs nothing.
func TestSpritesSnapshotRoundTrip(t *testing.T) {
	t.Parallel()

	const token = "snapshot-it-token" // #nosec G101 -- test fixture
	fake := newCheckpointFake(token)
	p := sprites.NewWithClient(fake, token)
	ctx := context.Background()

	id, err := p.Create(ctx, sandbox.Spec{Name: spriteName, Labels: []string{"kaxa", "integration"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	original, err := p.Snapshot(ctx, id, "integration original")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	changed, err := p.Snapshot(ctx, id, "integration changed")
	if err != nil {
		t.Fatalf("second snapshot: %v", err)
	}

	list, err := p.ListSnapshots(ctx, id)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].ID != changed.ID || list[1].ID != original.ID {
		t.Fatalf("list = %+v, want the two checkpoints newest first and no Current", list)
	}

	if got, err := p.Restore(ctx, sandbox.RestoreRequest{Sandbox: id, Snapshot: original.ID}); err != nil || got != id {
		t.Fatalf("restore = %q, %v", got, err)
	}
	if want := []string{string(original.ID)}; !slices.Equal(fake.restored, want) {
		t.Fatalf("restored = %v, want %v", fake.restored, want)
	}

	for _, snap := range []sandbox.Snapshot{changed, original} {
		if err := p.DeleteSnapshot(ctx, id, snap.ID); err != nil {
			t.Fatalf("delete %s: %v", snap.ID, err)
		}
	}
	if want := []string{string(changed.ID), string(original.ID)}; !slices.Equal(fake.deleted, want) {
		t.Fatalf("deleted = %v, want %v", fake.deleted, want)
	}
	if list, _ := p.ListSnapshots(ctx, id); len(list) != 0 {
		t.Fatalf("list after delete = %+v", list)
	}

	if err := p.Destroy(ctx, id); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if !slices.Equal(fake.destroyed, []string{spriteName}) {
		t.Fatalf("destroyed = %v", fake.destroyed)
	}
}
