package vercel

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestComputeOptions(t *testing.T) {
	t.Parallel()
	got, ok := sandbox.ComputeOptionsOf(New())
	if !ok || len(got.CPUs) != 4 || got.CPUs[0] != 1 || got.CPUs[3] != 8 || !got.CustomCPU || got.CustomMemory || len(got.MemoryMiB) != 0 {
		t.Fatalf("ComputeOptions() = %+v, %v", got, ok)
	}
	got.CPUs[0] = 99
	if again, _ := sandbox.ComputeOptionsOf(New()); again.CPUs[0] != 1 {
		t.Fatal("ComputeOptions shares its list with the caller")
	}
}

func TestListTemplates(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	now := time.Now().UTC()
	f.mu.Lock()
	f.snaps = []*fakeSnap{
		{id: "snap_old", sandbox: "a", status: snapshotCreated, method: snapshotManual, created: now.Add(-time.Hour)},
		{id: "snap_new", sandbox: "b", status: snapshotCreated, method: snapshotManual, created: now},
		{id: "snap_auto", sandbox: "a", status: snapshotCreated, method: methodAutomatic, created: now},
		{id: "snap_gone", sandbox: "a", status: "deleted", method: snapshotManual, created: now},
	}
	f.mu.Unlock()
	idx, err := p.index()
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.add(snapshotEntry{ID: "snap_new", Sandbox: "b", Comment: "after setup"}); err != nil {
		t.Fatal(err)
	}

	got, err := p.ListTemplates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "snap_new" || got[1].Name != "snap_old" {
		t.Fatalf("ListTemplates() = %+v, want the manual snapshots, newest first", got)
	}
	if !strings.Contains(got[0].Label, "snap_new") || !strings.Contains(got[0].Label, "after setup") {
		t.Fatalf("label = %q, want the id and the comment", got[0].Label)
	}
	if strings.Contains(got[1].Label, "()") {
		t.Fatalf("label = %q, want no empty brackets", got[1].Label)
	}
}

func TestListTemplatesErrors(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	f.fail2("GET /v2/sandboxes/snapshots", 403)
	_, err := p.ListTemplates(context.Background())
	if err == nil || !strings.Contains(err.Error(), "rejected") || strings.Contains(err.Error(), testToken) {
		t.Fatalf("ListTemplates() = %v", err)
	}
	if _, err := New().ListTemplates(context.Background()); err == nil {
		t.Fatal("an unconfigured provider listed templates")
	}
}

func TestListTemplatesToleratesBrokenIndex(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	f.mu.Lock()
	f.snaps = []*fakeSnap{{id: "snap_a", sandbox: "a", status: snapshotCreated, method: snapshotManual, created: time.Now()}}
	f.mu.Unlock()
	p.SetSnapshotIndexPath(t.TempDir()) // a directory, so the index cannot be read
	got, err := p.ListTemplates(context.Background())
	if err != nil || len(got) != 1 {
		t.Fatalf("ListTemplates() = %+v, %v; comments are optional", got, err)
	}
}
