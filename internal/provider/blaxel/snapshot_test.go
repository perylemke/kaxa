package blaxel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestSnapshotCreateListRestoreDelete(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
	api.add(boxName, statusRunning)

	got, err := p.Snapshot(t.Context(), boxName, "before the change")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if got.Provider != ProviderName || got.Sandbox != boxName || got.Comment != "before the change" || got.ID == "" {
		t.Fatalf("snapshot = %+v", got)
	}

	listed, err := p.ListSnapshots(t.Context(), boxName)
	if err != nil || len(listed) != 1 || listed[0].ID != got.ID || listed[0].Comment != got.Comment {
		t.Fatalf("list = %+v, %v", listed, err)
	}
	all, err := p.ListAllSnapshots(t.Context())
	if err != nil || len(all) != 1 {
		t.Fatalf("catalog = %+v, %v", all, err)
	}
}

func TestSnapshotCatalogSkipsUnrecorded(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
	api.add(boxName, statusRunning)
	got, err := p.Snapshot(t.Context(), boxName, "kept")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	api.snaps = append(api.snaps, snapshotRow{
		ID: "foreign", Name: "foreign", Sandbox: boxName, Status: snapReady, CreatedAt: time.Unix(1, 0).UTC(),
	})
	all, err := p.ListAllSnapshots(t.Context())
	if err != nil || len(all) != 1 || all[0].ID != got.ID {
		t.Fatalf("catalog included a snapshot kaxa did not record: %+v, %v", all, err)
	}
}

func TestSnapshotRestoreAndDelete(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
	api.add(boxName, statusRunning)
	got, err := p.Snapshot(t.Context(), boxName, "")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	id, err := p.Restore(t.Context(), sandbox.RestoreRequest{Sandbox: boxName, Snapshot: got.ID})
	if err != nil || id != boxName {
		t.Fatalf("restore = %q, %v", id, err)
	}
	if _, err := p.Restore(t.Context(), sandbox.RestoreRequest{Sandbox: boxName, Snapshot: got.ID, Target: "copy"}); err == nil {
		t.Fatal("in-place restore accepted a target")
	}
	if err := p.DeleteSnapshot(t.Context(), boxName, got.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	all, err := p.ListAllSnapshots(t.Context())
	if err != nil || len(all) != 0 {
		t.Fatalf("catalog after delete = %+v, %v", all, err)
	}
}

func TestSnapshotWaitsUntilReady(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	api.snapPending = 1
	p := newTestProvider(t, api)
	api.add(boxName, statusRunning)
	got, err := p.Snapshot(t.Context(), boxName, "")
	if err != nil || got.ID == "" {
		t.Fatalf("snapshot = %+v, %v", got, err)
	}
}

func TestSnapshotFailed(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	api.snapFail = true
	p := newTestProvider(t, api)
	api.add(boxName, statusRunning)
	_, err := p.Snapshot(t.Context(), boxName, "")
	if err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("err = %v", err)
	}
}

func TestSnapshotRejectsBadComment(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
	api.add(boxName, statusRunning)
	_, err := p.Snapshot(t.Context(), boxName, "bad\x07comment")
	if err == nil {
		t.Fatal("control character accepted")
	}
}

func TestRestoreRejectsPendingAndBadID(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
	api.add(boxName, statusRunning)
	api.snaps = append(api.snaps, snapshotRow{ID: "snap-pending", Sandbox: boxName, Status: "pending"})
	_, err := p.Restore(t.Context(), sandbox.RestoreRequest{Sandbox: boxName, Snapshot: "snap-pending"})
	if err == nil || !strings.Contains(err.Error(), "cannot be restored") {
		t.Fatalf("pending err = %v", err)
	}
	_, err = p.Restore(t.Context(), sandbox.RestoreRequest{Sandbox: boxName, Snapshot: "../x"})
	if err == nil || !strings.Contains(err.Error(), "invalid snapshot id") {
		t.Fatalf("id err = %v", err)
	}
}

func TestSnapshotIndexRejectsBadFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "blaxel-snapshots.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newSnapshotIndex(path).all(); err == nil {
		t.Fatal("broken json accepted")
	}
	if err := os.WriteFile(path, []byte(`{"version":2,"snapshots":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newSnapshotIndex(path).all(); err == nil {
		t.Fatal("unsupported version accepted")
	}
}

func TestDefaultSnapshotIndexPath(t *testing.T) {
	t.Parallel()
	path, err := defaultIndexPath()
	if err != nil || !strings.HasSuffix(path, "blaxel-snapshots.json") {
		t.Fatalf("path = %q, %v", path, err)
	}
}

func TestRestoreMissingSnapshot(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
	api.add(boxName, statusRunning)
	_, err := p.Restore(t.Context(), sandbox.RestoreRequest{Sandbox: boxName, Snapshot: "missing"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestDiskPercentOnCreate(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
	if err := p.SetDiskPercent(60); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Create(t.Context(), sandbox.Spec{Name: boxName}); err != nil {
		t.Fatal(err)
	}
	if api.lastCreate.DiskPercent != 60 {
		t.Fatalf("disk = %d, want 60", api.lastCreate.DiskPercent)
	}
	if _, err := p.Create(t.Context(), sandbox.Spec{Name: "other", DiskPercent: 80}); err != nil {
		t.Fatal(err)
	}
	if api.lastCreate.DiskPercent != 80 {
		t.Fatalf("disk = %d, want 80", api.lastCreate.DiskPercent)
	}
	if err := p.SetDiskPercent(5); err == nil {
		t.Fatal("disk percent 5 accepted")
	}
}

func TestUnarchiveArchivedSandbox(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
	api.add(boxName, statusArchived)
	if err := p.Unarchive(t.Context(), boxName); err != nil {
		t.Fatalf("unarchive: %v", err)
	}
	rec, err := api.Find(t.Context(), boxName)
	if err != nil || rec.Status != statusRunning {
		t.Fatalf("after unarchive = %+v, %v", rec, err)
	}
	if err := p.Unarchive(t.Context(), boxName); err == nil || !strings.Contains(err.Error(), "running") {
		t.Fatalf("second unarchive err = %v", err)
	}
}
