package modal_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/provider/modal"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	firstImage = "im-1"
	srcName    = "src"
	newTarget  = "new"
)

func newSnapshotProvider(t *testing.T, fake *fakeClient) *modal.Provider {
	t.Helper()
	p := newProvider(t, fake)
	p.SetSnapshotIndexPath(filepath.Join(t.TempDir(), "kaxa", "snapshots.json"))
	return p
}

func addBox(t *testing.T, p *modal.Provider, name string, labels ...string) {
	t.Helper()
	if _, err := p.Create(context.Background(), sandbox.Spec{Name: name, Labels: labels}); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
}

func TestSnapshotRestoreKeepsSyncLabel(t *testing.T) {
	t.Parallel()

	fake := newFakeClient()
	p := newSnapshotProvider(t, fake)
	addBox(t, p, srcName, sandbox.SyncLabel, "kaxa")
	ctx := context.Background()

	snap, err := p.Snapshot(ctx, srcName, "before upgrade")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.ID != firstImage || snap.Comment != "before upgrade" || snap.Sandbox != srcName || snap.Provider != testProviderName || snap.CreatedAt.IsZero() {
		t.Fatalf("snapshot = %+v", snap)
	}

	got, err := p.Restore(ctx, sandbox.RestoreRequest{Sandbox: srcName, Snapshot: snap.ID, Target: "copy"})
	if err != nil || got != "copy" {
		t.Fatalf("restore = %q, %v", got, err)
	}
	assertRestoredWithSync(t, fake)
	if _, ok := fake.boxes[srcName]; !ok {
		t.Fatal("source sandbox must be kept")
	}
}

func assertRestoredWithSync(t *testing.T, fake *fakeClient) {
	t.Helper()
	if len(fake.fromImage) != 1 {
		t.Fatalf("fromImage = %+v", fake.fromImage)
	}
	call := fake.fromImage[0]
	if call.name != "copy" || call.image != firstImage || !slices.Contains(call.labels, sandbox.SyncLabel) {
		t.Fatalf("restore call = %+v, want sync label on copy", call)
	}
}

func TestListSnapshotsNewestFirstAndAfterDestroy(t *testing.T) {
	t.Parallel()

	fake := newFakeClient()
	p := newSnapshotProvider(t, fake)
	addBox(t, p, srcName)
	addBox(t, p, "other")
	ctx := context.Background()

	for _, c := range []string{"one", "two"} {
		if _, err := p.Snapshot(ctx, srcName, c); err != nil {
			t.Fatalf("snapshot: %v", err)
		}
	}
	if _, err := p.Snapshot(ctx, "other", "x"); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if err := p.Destroy(ctx, srcName); err != nil {
		t.Fatalf("destroy: %v", err)
	}

	list, err := p.ListSnapshots(ctx, srcName)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].Comment != "two" || list[1].Comment != "one" {
		t.Fatalf("list = %+v", list)
	}
}

func TestRestoreErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  sandbox.RestoreRequest
		want string
	}{
		{"target required", sandbox.RestoreRequest{Sandbox: srcName, Snapshot: firstImage}, "invalid sandbox name"},
		{"same target", sandbox.RestoreRequest{Sandbox: srcName, Snapshot: firstImage, Target: srcName}, "differ"},
		{"target exists", sandbox.RestoreRequest{Sandbox: srcName, Snapshot: firstImage, Target: "taken"}, "already exists"},
		{"unknown snapshot", sandbox.RestoreRequest{Sandbox: srcName, Snapshot: "im-99", Target: newTarget}, "not found"},
		{"bad snapshot id", sandbox.RestoreRequest{Sandbox: srcName, Snapshot: "../x", Target: newTarget}, "invalid snapshot id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newSnapshotProvider(t, newFakeClient())
			addBox(t, p, srcName)
			addBox(t, p, "taken")
			if _, err := p.Snapshot(context.Background(), srcName, ""); err != nil {
				t.Fatalf("snapshot: %v", err)
			}
			_, err := p.Restore(context.Background(), tt.req)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestListAllSnapshotsIncludesDestroyedSandboxes(t *testing.T) {
	t.Parallel()

	p := newSnapshotProvider(t, newFakeClient())
	addBox(t, p, srcName)
	addBox(t, p, "other")
	ctx := context.Background()
	for _, name := range []sandbox.ID{srcName, "other", srcName} {
		if _, err := p.Snapshot(ctx, name, string(name)); err != nil {
			t.Fatalf("snapshot: %v", err)
		}
	}
	if err := p.Destroy(ctx, srcName); err != nil {
		t.Fatalf("destroy: %v", err)
	}

	all, err := p.ListAllSnapshots(ctx)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("list all = %+v, want the three snapshots", all)
	}
	for _, s := range all {
		if s.Provider != testProviderName || s.Sandbox == "" || s.ID == "" || s.CreatedAt.IsZero() {
			t.Fatalf("snapshot = %+v, want provider, sandbox, id and time", s)
		}
	}
}

var _ sandbox.SnapshotCatalog = (*modal.Provider)(nil)

func TestListAllSnapshotsEmpty(t *testing.T) {
	t.Parallel()

	all, err := newSnapshotProvider(t, newFakeClient()).ListAllSnapshots(context.Background())
	if err != nil || len(all) != 0 {
		t.Fatalf("list all = %+v, err = %v", all, err)
	}
}

func TestListAllSnapshotsNeedsNoCredentials(t *testing.T) {
	t.Parallel()

	// The index is local, so listing it works before any API call.
	p := modal.New()
	p.SetSnapshotIndexPath(filepath.Join(t.TempDir(), "snapshots.json"))
	if _, err := p.ListAllSnapshots(context.Background()); err != nil {
		t.Fatalf("list all: %v", err)
	}
}

// The hint must be a command that works: snapshot list needs the sandbox name.
func TestUnknownSnapshotHintNamesTheSandbox(t *testing.T) {
	t.Parallel()

	p := newSnapshotProvider(t, newFakeClient())
	addBox(t, p, srcName)
	_, err := p.Restore(context.Background(), sandbox.RestoreRequest{Sandbox: srcName, Snapshot: "im-99", Target: newTarget})
	ue, ok := usererr.As(err)
	if !ok {
		t.Fatalf("err = %v, want a user error", err)
	}
	if want := "kaxa snapshot list " + srcName + " --provider modal"; !strings.Contains(ue.Hint, want) {
		t.Fatalf("hint = %q, want %q", ue.Hint, want)
	}
}

func TestRestoreRefusesSnapshotOfAnotherSandbox(t *testing.T) {
	t.Parallel()

	fake := newFakeClient()
	p := newSnapshotProvider(t, fake)
	addBox(t, p, "a")
	addBox(t, p, "b")
	snap, err := p.Snapshot(context.Background(), "a", "")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	_, err = p.Restore(context.Background(), sandbox.RestoreRequest{Sandbox: "b", Snapshot: snap.ID, Target: newTarget})
	if err == nil {
		t.Fatal("restored a snapshot that belongs to another sandbox")
	}
	if err := p.DeleteSnapshot(context.Background(), "b", snap.ID); err == nil {
		t.Fatal("deleted a snapshot that belongs to another sandbox")
	}
	if len(fake.deletedImages) != 0 {
		t.Fatalf("deleted images = %v", fake.deletedImages)
	}
}

// brokenIndexProvider returns a provider whose snapshot index cannot be
// written: its directory would have to live under a regular file.
func brokenIndexProvider(t *testing.T, fake *fakeClient) *modal.Provider {
	t.Helper()
	p := newProvider(t, fake)
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	p.SetSnapshotIndexPath(filepath.Join(file, "kaxa", "snapshots.json"))
	return p
}

func TestSnapshotIndexFailureDeletesTheImage(t *testing.T) {
	t.Parallel()

	fake := newFakeClient()
	p := brokenIndexProvider(t, fake)
	addBox(t, p, srcName)
	if _, err := p.Snapshot(context.Background(), srcName, ""); err == nil {
		t.Fatal("snapshot succeeded without a writable index")
	}
	if !slices.Equal(fake.deletedImages, []string{firstImage}) {
		t.Fatalf("deleted images = %v, want the unrecorded image removed", fake.deletedImages)
	}
}

// An image kaxa could neither record nor delete keeps a copy of the filesystem
// for 30 days, so the error has to name it.
func TestSnapshotIndexFailureNamesAnImageItCannotDelete(t *testing.T) {
	t.Parallel()

	errDelete := errors.New("modal is down")
	fake := newFakeClient()
	fake.deleteImageErr = errDelete
	p := brokenIndexProvider(t, fake)
	addBox(t, p, srcName)
	_, err := p.Snapshot(context.Background(), srcName, "")
	if err == nil {
		t.Fatal("snapshot succeeded without a writable index")
	}
	if !strings.Contains(err.Error(), firstImage) || !strings.Contains(err.Error(), errDelete.Error()) {
		t.Fatalf("err = %q, want the image id and the delete error", err)
	}
	if !errors.Is(err, errDelete) {
		t.Fatalf("err = %v, want it to wrap the delete error", err)
	}
}

func TestDeleteSnapshot(t *testing.T) {
	t.Parallel()

	t.Run("removes image and entry", func(t *testing.T) {
		t.Parallel()
		fake := newFakeClient()
		p := newSnapshotProvider(t, fake)
		addBox(t, p, srcName)
		snap, err := p.Snapshot(context.Background(), srcName, "")
		if err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		if err := p.DeleteSnapshot(context.Background(), srcName, snap.ID); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if !slices.Equal(fake.deletedImages, []string{firstImage}) {
			t.Fatalf("deleted images = %v", fake.deletedImages)
		}
		list, _ := p.ListSnapshots(context.Background(), srcName)
		if len(list) != 0 {
			t.Fatalf("list after delete = %+v", list)
		}
	})

	t.Run("image already gone still clears the entry", func(t *testing.T) {
		t.Parallel()
		fake := newFakeClient()
		p := newSnapshotProvider(t, fake)
		addBox(t, p, srcName)
		snap, err := p.Snapshot(context.Background(), srcName, "")
		if err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		delete(fake.images, string(snap.ID))
		if err := p.DeleteSnapshot(context.Background(), srcName, snap.ID); err != nil {
			t.Fatalf("delete: %v", err)
		}
		list, _ := p.ListSnapshots(context.Background(), srcName)
		if len(list) != 0 {
			t.Fatalf("list after delete = %+v", list)
		}
	})
}

func TestRestoreExpiredImageDropsEntry(t *testing.T) {
	t.Parallel()

	fake := newFakeClient()
	p := newSnapshotProvider(t, fake)
	addBox(t, p, srcName)
	snap, err := p.Snapshot(context.Background(), srcName, "")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	delete(fake.images, string(snap.ID))

	_, err = p.Restore(context.Background(), sandbox.RestoreRequest{Sandbox: srcName, Snapshot: snap.ID, Target: newTarget})
	if !errors.Is(err, modal.ErrImageNotFound) {
		t.Fatalf("err = %v, want ErrImageNotFound", err)
	}
	list, _ := p.ListSnapshots(context.Background(), srcName)
	if len(list) != 0 {
		t.Fatalf("stale entry kept: %+v", list)
	}
}

func TestSnapshotErrors(t *testing.T) {
	t.Parallel()

	p := newSnapshotProvider(t, newFakeClient())
	ctx := context.Background()

	if _, err := p.Snapshot(ctx, "missing", ""); !errors.Is(err, modal.ErrNotFound) {
		t.Fatalf("missing sandbox err = %v", err)
	}
	if _, err := p.Snapshot(ctx, "Bad_Name", ""); err == nil {
		t.Fatal("snapshot accepted a bad name")
	}
	addBox(t, p, srcName)
	if _, err := p.Snapshot(ctx, srcName, "a\nb"); err == nil {
		t.Fatal("snapshot accepted a comment with a control character")
	}
}

func TestSnapshotRedactsSecrets(t *testing.T) {
	t.Parallel()

	fake := newFakeClient()
	p := newSnapshotProvider(t, fake)
	addBox(t, p, srcName)
	fake.failWith = errors.New("denied for " + testTokenSecret)

	_, err := p.Snapshot(context.Background(), srcName, "")
	if err == nil || strings.Contains(err.Error(), testTokenSecret) {
		t.Fatalf("err = %v, want redacted error", err)
	}
}

func TestSnapshotNeedsCredentials(t *testing.T) {
	t.Parallel()

	p := modal.New()
	p.SetSnapshotIndexPath(filepath.Join(t.TempDir(), "snapshots.json"))
	if _, err := p.ListSnapshots(context.Background(), srcName); err != nil {
		t.Fatalf("list reads only the index and should not need credentials: %v", err)
	}
	if _, err := p.Snapshot(context.Background(), srcName, ""); !errors.Is(err, sandbox.ErrNotConfigured) {
		t.Fatalf("snapshot err = %v, want ErrNotConfigured", err)
	}
}
