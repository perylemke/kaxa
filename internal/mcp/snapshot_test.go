package mcp

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func (f *fakeProvider) RestoreMode() sandbox.RestoreMode {
	if f.restoreMode != "" {
		return f.restoreMode
	}
	return sandbox.RestoreInPlace
}

func (f *fakeProvider) Snapshot(_ context.Context, id sandbox.ID, comment string) (sandbox.Snapshot, error) {
	snap := sandbox.Snapshot{
		ID: "v1", Sandbox: id, Provider: f.name, Comment: comment,
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	f.snaps = append(f.snaps, snap)
	return snap, f.snapErr
}

func (f *fakeProvider) ListSnapshots(context.Context, sandbox.ID) ([]sandbox.Snapshot, error) {
	return f.snaps, nil
}

func (f *fakeProvider) Restore(_ context.Context, req sandbox.RestoreRequest) (sandbox.ID, error) {
	f.restores = append(f.restores, req)
	if req.Target != "" {
		return req.Target, nil
	}
	return req.Sandbox, nil
}

func (f *fakeProvider) DeleteSnapshot(_ context.Context, _ sandbox.ID, snap sandbox.SnapshotID) error {
	f.deletedSnaps = append(f.deletedSnaps, snap)
	return nil
}

func TestSnapshotToolsRoundTrip(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testSprites, infos: []sandbox.Info{{ID: testBox}}}
	svc := newTestService(t, t.TempDir(), testSprites, fp)
	ctx := context.Background()

	created, err := svc.createSnapshot(ctx, createSnapshotIn{Name: testBox, Comment: "before"})
	if err != nil || created.ID != "v1" || created.Comment != "before" || created.Provider != testSprites || created.CreatedAt != "2026-01-02T03:04:05Z" {
		t.Fatalf("created = %+v err = %v", created, err)
	}
	listed, err := svc.listSnapshots(ctx, listSnapshotsIn{Name: testBox})
	if err != nil || len(listed.Snapshots) != 1 || listed.Snapshots[0].ID != "v1" {
		t.Fatalf("listed = %+v err = %v", listed, err)
	}
	restoreAndDelete(t, svc, fp)
}

func restoreAndDelete(t *testing.T, svc *service, fp *fakeProvider) {
	t.Helper()
	ctx := context.Background()
	restored, err := svc.restoreSnapshot(ctx, restoreSnapshotIn{Name: testBox, Snapshot: "v1"})
	if err != nil || restored.Status != "restored" || restored.Sandbox != testBox {
		t.Fatalf("restored = %+v err = %v", restored, err)
	}
	deleted, err := svc.deleteSnapshot(ctx, deleteSnapshotIn{Name: testBox, Snapshot: "v1"})
	if err != nil || deleted.Status != "deleted" || !slices.Equal(fp.deletedSnaps, []sandbox.SnapshotID{"v1"}) {
		t.Fatalf("deleted = %+v err = %v", deleted, err)
	}
}

func TestListSnapshotsEmptyIsNotNull(t *testing.T) {
	t.Parallel()

	svc := newTestService(t, t.TempDir(), testSprites, &fakeProvider{name: testSprites, infos: []sandbox.Info{{ID: testBox}}})
	out, err := svc.listSnapshots(context.Background(), listSnapshotsIn{Name: testBox})
	if err != nil || out.Snapshots == nil {
		t.Fatalf("out = %+v err = %v", out, err)
	}
}

func TestRestoreSnapshotTargetByMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mode    sandbox.RestoreMode
		target  string
		wantErr bool
		wantID  string
	}{
		{"in place without target", sandbox.RestoreInPlace, "", false, testBox},
		{"in place rejects target", sandbox.RestoreInPlace, "copy", true, ""},
		{"new sandbox with target", sandbox.RestoreNewSandbox, "copy", false, "copy"},
		{"new sandbox needs target", sandbox.RestoreNewSandbox, "", true, ""},
		{"new sandbox same target", sandbox.RestoreNewSandbox, testBox, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fp := &fakeProvider{name: testSprites, restoreMode: tt.mode}
			svc := newTestService(t, t.TempDir(), testSprites, fp)

			out, err := svc.restoreSnapshot(context.Background(), restoreSnapshotIn{Name: testBox, Snapshot: "v1", Target: tt.target})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if len(fp.restores) != 0 {
					t.Fatal("restore ran despite the invalid request")
				}
				return
			}
			if out.Sandbox != tt.wantID {
				t.Fatalf("sandbox = %q, want %q", out.Sandbox, tt.wantID)
			}
		})
	}
}

func TestSnapshotToolsValidateInput(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testSprites}
	svc := newTestService(t, t.TempDir(), testSprites, fp)
	ctx := context.Background()

	if _, err := svc.createSnapshot(ctx, createSnapshotIn{}); err == nil {
		t.Error("create_snapshot accepted an empty name")
	}
	if _, err := svc.restoreSnapshot(ctx, restoreSnapshotIn{Name: testBox, Snapshot: "../x"}); err == nil {
		t.Error("restore_snapshot accepted a path as snapshot id")
	}
	if _, err := svc.deleteSnapshot(ctx, deleteSnapshotIn{Name: testBox, Snapshot: "a/b"}); err == nil {
		t.Error("delete_snapshot accepted a path as snapshot id")
	}
	if len(fp.restores)+len(fp.deletedSnaps) != 0 {
		t.Error("an operation ran despite invalid input")
	}
}

func TestSnapshotToolsNeedCapability(t *testing.T) {
	t.Parallel()

	svc := newTestService(t, t.TempDir(), testPlain,
		plainProvider{name: testPlain}, &fakeProvider{name: testSprites})
	ctx := context.Background()

	_, err := svc.createSnapshot(ctx, createSnapshotIn{Name: testBox})
	wantErr(t, err, "snapshot is not available for plain")
	if !errors.Is(err, sandbox.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
	if _, err := svc.listSnapshots(ctx, listSnapshotsIn{Provider: testPlain, Name: testBox}); !errors.Is(err, sandbox.ErrUnsupported) {
		t.Errorf("list err = %v", err)
	}
	if _, err := svc.restoreSnapshot(ctx, restoreSnapshotIn{Name: testBox, Snapshot: "v1"}); !errors.Is(err, sandbox.ErrUnsupported) {
		t.Errorf("restore err = %v", err)
	}
	if _, err := svc.deleteSnapshot(ctx, deleteSnapshotIn{Name: testBox, Snapshot: "v1"}); !errors.Is(err, sandbox.ErrUnsupported) {
		t.Errorf("delete err = %v", err)
	}
}

func TestListProvidersReportsSnapshotRestoreMode(t *testing.T) {
	t.Parallel()

	svc := newTestService(t, t.TempDir(), testSprites,
		&fakeProvider{name: testSprites, restoreMode: sandbox.RestoreNewSandbox}, plainProvider{name: testPlain})
	out, err := svc.listProviders(context.Background(), listProvidersIn{})
	if err != nil {
		t.Fatalf("list providers: %v", err)
	}
	for _, p := range out.Providers {
		switch p.Name {
		case testSprites:
			if !p.Capabilities.Snapshot || p.Capabilities.SnapshotRestore != string(sandbox.RestoreNewSandbox) {
				t.Errorf("sprites capabilities = %+v", p.Capabilities)
			}
		case testPlain:
			if p.Capabilities.Snapshot {
				t.Errorf("plain capabilities = %+v", p.Capabilities)
			}
		}
	}
}

func snapshotFakes() (sprites, modal *fakeProvider) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	sprites = &fakeProvider{
		name: testSprites, infos: []sandbox.Info{{ID: "web"}},
		snaps: []sandbox.Snapshot{{ID: "v2", Comment: "first", CreatedAt: at}},
	}
	modal = &fakeProvider{
		name: testModal, infos: []sandbox.Info{{ID: "api"}},
		snaps: []sandbox.Snapshot{{ID: "im-1", Comment: "second", CreatedAt: at.Add(time.Hour)}},
	}
	return sprites, modal
}

func TestListSnapshotsAcrossProviders(t *testing.T) {
	t.Parallel()

	sprites, modal := snapshotFakes()
	svc := newTestService(t, t.TempDir(), testSprites, sprites, modal)
	ctx := context.Background()

	all, err := svc.listSnapshots(ctx, listSnapshotsIn{})
	if err != nil || len(all.Snapshots) != 2 || len(all.Warnings) != 0 {
		t.Fatalf("all = %+v err = %v", all, err)
	}
	// The registry lists providers by name, so modal comes first.
	want := []snapshotInfo{
		{ID: "im-1", Sandbox: "api", Provider: testModal, Comment: "second", CreatedAt: "2026-01-02T04:04:05Z"},
		{ID: "v2", Sandbox: "web", Provider: testSprites, Comment: "first", CreatedAt: "2026-01-02T03:04:05Z"},
	}
	if !slices.Equal(all.Snapshots, want) {
		t.Fatalf("snapshots = %+v, want %+v", all.Snapshots, want)
	}

	named, err := svc.listSnapshots(ctx, listSnapshotsIn{Name: "web"})
	if err != nil || len(named.Snapshots) != 1 || named.Snapshots[0].ID != "v2" {
		t.Fatalf("named = %+v err = %v", named, err)
	}
	only, err := svc.listSnapshots(ctx, listSnapshotsIn{Provider: testModal})
	if err != nil || len(only.Snapshots) != 1 || only.Snapshots[0].ID != "im-1" {
		t.Fatalf("only = %+v err = %v", only, err)
	}
}

func TestListSnapshotsOneProviderFailing(t *testing.T) {
	t.Parallel()

	sprites, modal := snapshotFakes()
	modal.listErr = errors.New("modal is down")
	svc := newTestService(t, t.TempDir(), testSprites, sprites, modal)

	out, err := svc.listSnapshots(context.Background(), listSnapshotsIn{})
	if err != nil || len(out.Snapshots) != 1 || out.Snapshots[0].ID != "v2" {
		t.Fatalf("out = %+v err = %v", out, err)
	}
	if len(out.Warnings) != 1 || !strings.Contains(out.Warnings[0], "could not list snapshots on modal") {
		t.Fatalf("warnings = %v", out.Warnings)
	}
}

func TestListSnapshotsAllFailing(t *testing.T) {
	t.Parallel()

	sprites, modal := snapshotFakes()
	sprites.listErr, modal.listErr = errors.New("sprites is down"), errors.New("modal is down")
	svc := newTestService(t, t.TempDir(), testSprites, sprites, modal)

	_, err := svc.listSnapshots(context.Background(), listSnapshotsIn{})
	if err == nil || !strings.Contains(err.Error(), "sprites is down") || !strings.Contains(err.Error(), "modal is down") {
		t.Fatalf("err = %v", err)
	}
}

func TestListSnapshotsUnknownProvider(t *testing.T) {
	t.Parallel()

	svc := newTestService(t, t.TempDir(), testSprites, &fakeProvider{name: testSprites})
	_, err := svc.listSnapshots(context.Background(), listSnapshotsIn{Provider: "nope"})
	wantErr(t, err, `unknown provider "nope"`)
}
