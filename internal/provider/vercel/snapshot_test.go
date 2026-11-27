package vercel

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

//nolint:gocyclo // one scenario that checks many fields of a single response
func TestSnapshotListsOnlyManual(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	f.pageSize = 1
	createBox(t, p)
	ctx := context.Background()
	first, err := p.Snapshot(ctx, boxName, "  before change ")
	if err != nil {
		t.Fatal(err)
	}
	if first.Provider != ProviderName || first.Sandbox != boxName || first.Comment != "before change" || first.CreatedAt.IsZero() {
		t.Fatalf("Snapshot() = %+v", first)
	}
	if f.body("POST /v3/sandboxes/sessions/{sid}/snapshot", 0)["expiration"] != 0.0 {
		t.Fatal("manual snapshot can expire")
	}
	// The snapshot ended the session; the next command resumes the sandbox,
	// and stopping it again leaves an automatic snapshot that must stay hidden.
	second, err := p.Snapshot(ctx, boxName, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Exec(ctx, boxName, sandbox.ExecOptions{Command: []string{tagTrue}}); err != nil {
		t.Fatal(err)
	}
	if err := p.Restart(ctx, boxName); err != nil {
		t.Fatal(err)
	}
	list, err := p.ListSnapshots(ctx, boxName)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != second.ID || list[1].ID != first.ID || list[1].Comment != "before change" {
		t.Fatalf("ListSnapshots() = %+v", list)
	}
	f.mu.Lock()
	if len(f.snaps) < 3 {
		t.Fatalf("expected an automatic snapshot, have %d", len(f.snaps))
	}
	f.mu.Unlock()
}

func TestSnapshotRejects(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	ctx := context.Background()
	if _, err := p.Snapshot(ctx, boxName, "bad\ncomment"); err == nil {
		t.Fatal("control character accepted")
	}
	if _, err := p.Snapshot(ctx, "missing", ""); err == nil {
		t.Fatal("missing sandbox accepted")
	}
	f.fail2("POST /v3/sandboxes/sessions/{sid}/snapshot", 500)
	if _, err := p.Snapshot(ctx, boxName, ""); err == nil {
		t.Fatal("API failure ignored")
	}
	if _, err := p.ListSnapshots(ctx, "Bad Name"); err == nil {
		t.Fatal("bad name accepted")
	}
	f.fail2("GET /v2/sandboxes/snapshots", 500)
	if _, err := p.ListSnapshots(ctx, boxName); err == nil {
		t.Fatal("list failure ignored")
	}
}

func TestRestoreInPlace(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	ctx := context.Background()
	snap, err := p.Snapshot(ctx, boxName, "")
	if err != nil {
		t.Fatal(err)
	}
	id, err := p.Restore(ctx, sandbox.RestoreRequest{Sandbox: boxName, Snapshot: snap.ID})
	if err != nil || id != boxName {
		t.Fatalf("Restore() = %q, %v", id, err)
	}
	sb := f.sandboxes[boxName]
	if sb.snapshot != string(snap.ID) || sb.status != statusRunning {
		t.Fatalf("sandbox = %+v", sb)
	}
	if got := f.body("PATCH /v2/sandboxes/{name}", 0)["currentSnapshotId"]; got != string(snap.ID) {
		t.Fatalf("currentSnapshotId = %v", got)
	}
}

func TestRestoreRejects(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	ctx := context.Background()
	snap, err := p.Snapshot(ctx, boxName, "")
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	autoID := f.nextID("snap")
	f.snaps = append(f.snaps, &fakeSnap{id: autoID, sandbox: boxName, status: snapshotCreated, method: methodAutomatic})
	f.mu.Unlock()
	for _, tt := range []struct {
		name string
		req  sandbox.RestoreRequest
	}{
		{"target", sandbox.RestoreRequest{Sandbox: boxName, Snapshot: snap.ID, Target: "other"}},
		{"bad id", sandbox.RestoreRequest{Sandbox: boxName, Snapshot: "../x"}},
		{"unknown", sandbox.RestoreRequest{Sandbox: boxName, Snapshot: "snap-999"}},
		{"automatic", sandbox.RestoreRequest{Sandbox: boxName, Snapshot: sandbox.SnapshotID(autoID)}},
		{"other sandbox", sandbox.RestoreRequest{Sandbox: "missing", Snapshot: snap.ID}},
		{"bad sandbox", sandbox.RestoreRequest{Sandbox: "Bad Name", Snapshot: snap.ID}},
	} {
		if _, err := p.Restore(ctx, tt.req); err == nil {
			t.Errorf("%s: Restore() accepted the request", tt.name)
		}
	}
	if f.calls("PATCH /v2/sandboxes/{name}") != 0 {
		t.Fatal("a rejected restore changed the sandbox")
	}
	f.fail2("PATCH /v2/sandboxes/{name}", 500)
	if _, err := p.Restore(ctx, sandbox.RestoreRequest{Sandbox: boxName, Snapshot: snap.ID}); err == nil {
		t.Fatal("patch failure ignored")
	}
}

func TestSnapshotNotFoundMessage(t *testing.T) {
	t.Parallel()
	p, _ := newTestProvider(t)
	createBox(t, p)
	_, err := p.Restore(context.Background(), sandbox.RestoreRequest{Sandbox: boxName, Snapshot: "snap-1"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v", err)
	}
}

func TestDeleteSnapshot(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	ctx := context.Background()
	snap, err := p.Snapshot(ctx, boxName, "keep me not")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.DeleteSnapshot(ctx, boxName, "../x"); err == nil {
		t.Fatal("bad id accepted")
	}
	if err := p.DeleteSnapshot(ctx, boxName, "snap-999"); err == nil {
		t.Fatal("unknown snapshot accepted")
	}
	f.fail2("DELETE /v2/sandboxes/snapshots/{id}", 500)
	if err := p.DeleteSnapshot(ctx, boxName, snap.ID); err == nil {
		t.Fatal("delete failure ignored")
	}
	delete(f.failures, "DELETE /v2/sandboxes/snapshots/{id}")
	if err := p.DeleteSnapshot(ctx, boxName, snap.ID); err != nil {
		t.Fatal(err)
	}
	list, err := p.ListSnapshots(ctx, boxName)
	if err != nil || len(list) != 0 {
		t.Fatalf("ListSnapshots() = %+v, %v", list, err)
	}
	entries, err := p.mustIndex(t).all()
	if err != nil || len(entries) != 0 {
		t.Fatalf("index = %+v, %v", entries, err)
	}
}

func (p *Provider) mustIndex(t *testing.T) *snapshotIndex {
	t.Helper()
	idx, err := p.index()
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

func TestDestroyForgetsComments(t *testing.T) {
	t.Parallel()
	p, _ := newTestProvider(t)
	createBox(t, p)
	ctx := context.Background()
	if _, err := p.Snapshot(ctx, boxName, "one"); err != nil {
		t.Fatal(err)
	}
	if err := p.Destroy(ctx, boxName); err != nil {
		t.Fatal(err)
	}
	if entries, err := p.mustIndex(t).all(); err != nil || len(entries) != 0 {
		t.Fatalf("index = %+v, %v", entries, err)
	}
}

func TestSnapshotIndexFailures(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	file := filepath.Join(dir, "plain")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name  string
		path  string
		write string
	}{
		{"corrupt", filepath.Join(dir, "corrupt.json"), "{nope"},
		{"version", filepath.Join(dir, "version.json"), `{"version": 9, "snapshots": []}`},
		{"unreadable", dir, ""},
		{"unwritable", filepath.Join(file, "idx.json"), ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, _ := newTestProvider(t)
			createBox(t, p)
			if tt.write != "" {
				if err := os.WriteFile(tt.path, []byte(tt.write), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			p.SetSnapshotIndexPath(tt.path)
			if _, err := p.Snapshot(ctx, boxName, "c"); err == nil {
				t.Fatal("Snapshot() ignored the index failure")
			}
			if tt.name != "unwritable" {
				if _, err := p.ListSnapshots(ctx, boxName); err == nil {
					t.Fatal("ListSnapshots() ignored the index failure")
				}
			}
		})
	}
}

func TestDeleteSnapshotIndexFailure(t *testing.T) {
	t.Parallel()
	p, _ := newTestProvider(t)
	createBox(t, p)
	ctx := context.Background()
	snap, err := p.Snapshot(ctx, boxName, "c")
	if err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	p.SetSnapshotIndexPath(bad)
	if err := p.DeleteSnapshot(ctx, boxName, snap.ID); err == nil || !strings.Contains(err.Error(), "comment") {
		t.Fatalf("error = %v", err)
	}
	if err := p.Destroy(ctx, boxName); err == nil {
		t.Fatal("Destroy() ignored the index failure")
	}
}

func TestDefaultIndexPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "cfg"))
	p, _ := newTestProvider(t)
	p.SetSnapshotIndexPath("")
	createBox(t, p)
	if _, err := p.Snapshot(context.Background(), boxName, "c"); err != nil {
		t.Fatal(err)
	}
	var found string
	_ = filepath.WalkDir(home, func(path string, _ os.DirEntry, _ error) error {
		if filepath.Base(path) == indexFile {
			found = path
		}
		return nil
	})
	if found == "" {
		t.Fatal("index file was not written under the config directory")
	}
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if _, err := p.index(); err == nil {
		t.Fatal("index() found a config directory without HOME")
	}
}
