package daytona

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestSnapshotNameRoundTrip(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 1, 18, 30, 5, 0, time.UTC)
	name := snapshotName("my-box", at)
	if name != "kaxa-my-box-20261101T183005Z" || !sandbox.ValidSnapshotID(sandbox.SnapshotID(name)) {
		t.Fatalf("name = %q", name)
	}
	owner, got, ok := parseSnapshotName(name)
	if !ok || owner != "my-box" || !got.Equal(at) {
		t.Fatalf("parse = %q, %v, %v", owner, got, ok)
	}
	for _, bad := range []string{"", "my-box", "kaxa-", "kaxa-box", "kaxa-box-2026", "kaxa-Box-20261101T183005Z", "other-box-20261101T183005Z", "kaxa-box_20261101T183005Z"} {
		if _, _, ok := parseSnapshotName(bad); ok {
			t.Errorf("parseSnapshotName(%q) accepted", bad)
		}
	}
}

func snapshotFixture(t *testing.T) (*Provider, *fakeAPI) {
	t.Helper()
	p, api := newTestProvider(t, newFakeToolbox(okHandler))
	api.add(boxName, statusRunning, sandbox.SyncLabel)
	return p, api
}

func TestSnapshotListRestoreDelete(t *testing.T) {
	t.Parallel()
	p, api := snapshotFixture(t)
	snap, err := p.Snapshot(t.Context(), boxName, "  before refactor ")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	expect(t, "snapshot", []any{snap.Sandbox, snap.Provider, snap.Comment, strings.HasPrefix(string(snap.ID), "kaxa-box-")},
		[]any{sandbox.ID(boxName), "daytona", "before refactor", true})

	list, err := p.ListSnapshots(t.Context(), boxName)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	expect(t, "ListSnapshots", list, []sandbox.Snapshot{snap})

	id, err := p.Restore(t.Context(), sandbox.RestoreRequest{Sandbox: boxName, Snapshot: snap.ID, Target: "copy"})
	if err != nil || id != "copy" {
		t.Fatalf("Restore = %q, %v", id, err)
	}
	copyRec, _ := api.Find(t.Context(), "copy")
	expect(t, "restored sandbox", []any{copyRec.Template, copyRec.Labels}, []any{string(snap.ID), []string{sandbox.SyncLabel}})
	if _, err := api.Find(t.Context(), boxName); err != nil {
		t.Fatalf("source sandbox lost: %v", err)
	}

	if err := p.DeleteSnapshot(t.Context(), boxName, snap.ID); err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}
	if list, _ := p.ListSnapshots(t.Context(), boxName); len(list) != 0 {
		t.Fatalf("snapshot not deleted: %+v", list)
	}
	if err := p.DeleteSnapshot(t.Context(), boxName, snap.ID); err == nil {
		t.Fatal("deleting a snapshot that no longer exists must report it as not found")
	}
}

func TestSnapshotsSurviveSandboxDestroy(t *testing.T) {
	t.Parallel()
	p, _ := snapshotFixture(t)
	snap, err := p.Snapshot(t.Context(), boxName, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Destroy(t.Context(), boxName); err != nil {
		t.Fatal(err)
	}
	all, err := p.ListAllSnapshots(t.Context())
	if err != nil || len(all) != 1 || all[0].Sandbox != boxName {
		t.Fatalf("ListAllSnapshots = %+v, %v", all, err)
	}
	if _, err := p.Restore(t.Context(), sandbox.RestoreRequest{Sandbox: boxName, Snapshot: snap.ID, Target: "again"}); err != nil {
		t.Fatalf("Restore after destroy: %v", err)
	}
}

func TestSnapshotListIgnoresForeignImages(t *testing.T) {
	t.Parallel()
	p, api := snapshotFixture(t)
	api.images["kaxa-weird"] = &Image{ID: "img_x", Name: "kaxa-weird", Status: readyState}
	api.images["team-base"] = &Image{ID: "img_y", Name: "team-base", Status: readyState}
	all, err := p.ListAllSnapshots(t.Context())
	if err != nil || len(all) != 0 {
		t.Fatalf("ListAllSnapshots = %+v, %v", all, err)
	}
}

func TestSnapshotOwnership(t *testing.T) {
	t.Parallel()
	p, api := snapshotFixture(t)
	api.add(otherName, statusRunning)
	snap, err := p.Snapshot(t.Context(), boxName, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Restore(t.Context(), sandbox.RestoreRequest{Sandbox: otherName, Snapshot: snap.ID, Target: "x"}); err == nil {
		t.Fatal("restored a snapshot of another sandbox")
	}
	if err := p.DeleteSnapshot(t.Context(), otherName, snap.ID); err == nil {
		t.Fatal("deleted a snapshot of another sandbox")
	}
	api.images["team-base"] = &Image{ID: "img_y", Name: "team-base", Status: readyState}
	if err := p.DeleteSnapshot(t.Context(), boxName, "team-base"); err == nil {
		t.Fatal("deleted an image kaxa did not take")
	}
	if api.has("deleteimage img_y") {
		t.Fatal("foreign image was deleted")
	}
}

func TestRestoreValidation(t *testing.T) {
	t.Parallel()
	p, api := snapshotFixture(t)
	api.add("taken", statusRunning)
	snap, _ := p.Snapshot(t.Context(), boxName, "")
	tests := []struct {
		name string
		req  sandbox.RestoreRequest
	}{
		{"same target", sandbox.RestoreRequest{Sandbox: boxName, Snapshot: snap.ID, Target: boxName}},
		{"existing target", sandbox.RestoreRequest{Sandbox: boxName, Snapshot: snap.ID, Target: "taken"}},
		{"bad target", sandbox.RestoreRequest{Sandbox: boxName, Snapshot: snap.ID, Target: "Bad Name"}},
		{"bad snapshot id", sandbox.RestoreRequest{Sandbox: boxName, Snapshot: "../x", Target: newName}},
		{"unknown snapshot", sandbox.RestoreRequest{Sandbox: boxName, Snapshot: "kaxa-box-20200101T000000Z", Target: newName}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := p.Restore(t.Context(), tt.req); err == nil {
				t.Fatal("Restore succeeded")
			}
		})
	}
}

func TestRestoreRejectsUnreadyImage(t *testing.T) {
	t.Parallel()
	p, api := snapshotFixture(t)
	snap, _ := p.Snapshot(t.Context(), boxName, "")
	api.images[string(snap.ID)].Status = "building"
	if _, err := p.Restore(t.Context(), sandbox.RestoreRequest{Sandbox: boxName, Snapshot: snap.ID, Target: newName}); err == nil {
		t.Fatal("restored an image that is not ready")
	}
	if _, err := api.Find(t.Context(), newName); !errors.Is(err, ErrNotFound) {
		t.Fatal("a sandbox was created")
	}
}

func TestSnapshotFailedImageIsRemoved(t *testing.T) {
	t.Parallel()
	p, api := snapshotFixture(t)
	api.failImage = true
	_, err := p.Snapshot(t.Context(), boxName, "")
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("err = %v", err)
	}
	if len(api.images) != 0 {
		t.Fatal("failed image was kept")
	}
}

func TestSnapshotCommentValidation(t *testing.T) {
	t.Parallel()
	p, api := snapshotFixture(t)
	for _, c := range []string{strings.Repeat("x", 300), "bad\x07comment"} {
		if _, err := p.Snapshot(t.Context(), boxName, c); err == nil {
			t.Errorf("comment %q accepted", c)
		}
	}
	if len(api.images) != 0 {
		t.Fatal("an image was captured")
	}
}

func TestSnapshotStartsStoppedSandbox(t *testing.T) {
	t.Parallel()
	p, api := newTestProvider(t, newFakeToolbox(okHandler))
	api.add(boxName, statusStopped)
	if _, err := p.Snapshot(t.Context(), boxName, ""); err != nil {
		t.Fatal(err)
	}
	if !api.has("start sb-1") {
		t.Fatalf("calls = %v", api.calls)
	}
}

func TestDeleteSnapshotConflict(t *testing.T) {
	t.Parallel()
	p, api := snapshotFixture(t)
	snap, _ := p.Snapshot(t.Context(), boxName, "")
	api.deleteErr = &APIError{Status: http.StatusConflict, Message: "in use"}
	err := p.DeleteSnapshot(t.Context(), boxName, snap.ID)
	if err == nil || !strings.Contains(err.Error(), "cannot delete") {
		t.Fatalf("err = %v", err)
	}
}

func TestSnapshotIndexFileIsPrivateAndValidated(t *testing.T) {
	t.Parallel()
	path := t.TempDir() + "/sub/idx.json"
	idx := newSnapshotIndex(path)
	if err := idx.add(snapshotEntry{Image: "kaxa-a-20260101T000000Z", Comment: "c"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != indexFileMode {
		t.Fatalf("mode = %v, %v", info.Mode().Perm(), err)
	}
	if err := idx.remove("missing"); err != nil {
		t.Fatalf("remove missing: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"version":99}`), indexFileMode); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.all(); err == nil {
		t.Fatal("unsupported version accepted")
	}
	if err := os.WriteFile(path, []byte(`not json`), indexFileMode); err != nil {
		t.Fatal(err)
	}
	if _, err := idx.all(); err == nil {
		t.Fatal("corrupt index accepted")
	}
}
