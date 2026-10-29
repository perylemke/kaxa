package modal

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestSnapshotIndexFilePermissions(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions")
	}

	dir := filepath.Join(t.TempDir(), "kaxa")
	idx := newSnapshotIndex(filepath.Join(dir, "idx.json"))
	if err := idx.add(snapshotEntry{ID: "im-1", Sandbox: "a", CreatedAt: time.Now()}); err != nil {
		t.Fatalf("add: %v", err)
	}
	for path, want := range map[string]os.FileMode{dir: 0o700, idx.path: 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode = %o, want %o", path, got, want)
		}
	}
	left, _ := filepath.Glob(filepath.Join(dir, ".modal-snapshots-*"))
	if len(left) != 0 {
		t.Errorf("temporary files left: %v", left)
	}
}

func TestSnapshotIndexPrunesExpired(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	idx := newSnapshotIndex(filepath.Join(t.TempDir(), "idx.json"))
	idx.now = func() time.Time { return now }
	for _, e := range []snapshotEntry{
		{ID: "old", Sandbox: "a", CreatedAt: now.Add(-snapshotRetention - time.Hour)},
		{ID: "fresh", Sandbox: "a", CreatedAt: now.Add(-snapshotRetention + time.Hour)},
	} {
		// Write through save so the test controls CreatedAt.
		entries, err := idx.load()
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if err := idx.save(append(entries, e)); err != nil {
			t.Fatalf("save: %v", err)
		}
	}
	// The old entry is pruned on the load inside save of the second entry, so
	// check through a fresh read that only the fresh one is visible.
	got, err := idx.forSandbox("a")
	if err != nil {
		t.Fatalf("forSandbox: %v", err)
	}
	if len(got) != 1 || got[0].ID != "fresh" {
		t.Fatalf("entries = %+v, want only fresh", got)
	}
}

func TestSnapshotIndexRejectsUnknownVersion(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "idx.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"snapshots":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newSnapshotIndex(path).load(); err == nil {
		t.Fatal("loaded an index with an unknown version")
	}
}

func TestSnapshotIndexMissingFileIsEmpty(t *testing.T) {
	t.Parallel()

	got, err := newSnapshotIndex(filepath.Join(t.TempDir(), "none.json")).load()
	if err != nil || len(got) != 0 {
		t.Fatalf("load = %v, %v", got, err)
	}
}
