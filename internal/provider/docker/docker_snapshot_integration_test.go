//go:build integration

package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// cloudSnap is a snapshot of the fake API. It is creating until it is read once.
type cloudSnap struct {
	name, description, status, source string
	labels                            map[string]string
}

func (f *fakeCloud) snapshotRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/sandboxes/{id}/snapshots", f.createSnapshot)
	mux.HandleFunc("GET /v1/snapshots", f.listSnapshots)
	mux.HandleFunc("GET /v1/snapshots/{id}", f.getSnapshot)
	mux.HandleFunc("DELETE /v1/snapshots/{id}", f.deleteSnapshot)
	mux.HandleFunc("POST /v1/snapshots/{id}/restore", f.restoreSnapshot)
}

func wireSnap(id string, s *cloudSnap, detail bool) string {
	desc := ""
	if detail {
		desc = fmt.Sprintf(`,"description":%q`, s.description)
	}
	return fmt.Sprintf(`{"uid":%q,"name":"snapshots/%s","displayName":%q,"status":%q,"etag":"\"1\"","createdAt":"2026-01-02T03:04:05Z"%s}`,
		id, id, s.name, s.status, desc)
}

func notFound(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"code":"notFound"}`))
}

func (f *fakeCloud) createSnapshot(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DisplayName, Description, CaptureMode string
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	defer f.mu.Unlock()
	src, ok := f.boxes[r.PathValue("id")]
	if !ok {
		notFound(w)
		return
	}
	if src.status != statusRunning || body.CaptureMode != captureDisk {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"failedPrecondition","message":"sandbox is not running"}`))
		return
	}
	id := fmt.Sprintf("snap%d", len(f.snaps)+1)
	s := &cloudSnap{name: body.DisplayName, description: body.Description, status: snapshotCreating, source: r.PathValue("id"), labels: src.labels}
	f.snaps[id] = s
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(wireSnap(id, s, true)))
}

func (f *fakeCloud) listSnapshots(w http.ResponseWriter, _ *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var items []string
	for id, s := range f.snaps {
		items = append(items, wireSnap(id, s, false))
	}
	_, _ = fmt.Fprintf(w, `{"snapshots":[%s]}`, strings.Join(items, ","))
}

func (f *fakeCloud) getSnapshot(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.snaps[r.PathValue("id")]
	if !ok {
		notFound(w)
		return
	}
	_, _ = w.Write([]byte(wireSnap(r.PathValue("id"), s, true)))
	s.status = snapshotReady
}

func (f *fakeCloud) deleteSnapshot(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.snaps[r.PathValue("id")]; !ok || r.Header.Get("If-Match") == "" {
		notFound(w)
		return
	}
	delete(f.snaps, r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeCloud) restoreSnapshot(w http.ResponseWriter, r *http.Request) {
	var body struct{ DisplayName string }
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.snaps[r.PathValue("id")]; !ok {
		notFound(w)
		return
	}
	f.n++
	id := fmt.Sprintf("s%d", f.n)
	// Like Docker, a restored sandbox has none of the labels of the source.
	b := &cloudBox{name: body.DisplayName, status: "starting", next: statusRunning, labels: map[string]string{}, etag: 1}
	f.boxes[id] = b
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(f.wire(id, b)))
}

func TestSnapshotFlow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p, f := newIntegrationProvider(t)
	if _, err := p.Create(ctx, sandbox.Spec{Name: testSandbox, Labels: []string{labelKaxa, sandbox.SyncLabel}}); err != nil {
		t.Fatalf("create: %v", err)
	}
	f.mu.Lock()
	f.snaps["theirs"] = &cloudSnap{name: foreignSnapshot, status: snapshotReady}
	f.mu.Unlock()

	snap, err := p.Snapshot(ctx, testSandbox, "before the migration")
	if err != nil || snap.Sandbox != testSandbox || snap.Comment != "before the migration" {
		t.Fatalf("snapshot = %+v err = %v", snap, err)
	}
	listed, err := p.ListSnapshots(ctx, testSandbox)
	if err != nil || len(listed) != 1 || listed[0].ID != snap.ID || listed[0].Comment != "before the migration" {
		t.Fatalf("list = %+v err = %v; the snapshot of the user must not be listed", listed, err)
	}

	if _, err := p.Restore(ctx, sandbox.RestoreRequest{Sandbox: testSandbox, Snapshot: snap.ID, Target: "copy"}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	copyInfo, err := p.Inspect(ctx, "copy")
	if err != nil || copyInfo.Status != statusRunning || !slices.Contains(copyInfo.Labels, sandbox.SyncLabel) {
		t.Fatalf("restored sandbox = %+v err = %v, want it running with the labels of the source", copyInfo, err)
	}
	infos, err := p.List(ctx)
	if err != nil || len(infos) != 2 {
		t.Fatalf("list = %+v err = %v, want the source and the restored sandbox", infos, err)
	}

	// The snapshot outlives its sandbox, and a restore needs only the name.
	if err := p.Destroy(ctx, testSandbox); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	all, err := p.ListAllSnapshots(ctx)
	if err != nil || len(all) != 1 || all[0].Sandbox != testSandbox {
		t.Fatalf("list all = %+v err = %v", all, err)
	}
	if err := p.DeleteSnapshot(ctx, testSandbox, "theirs"); err == nil {
		t.Fatal("a snapshot the user took must not be deleted")
	}
	if err := p.DeleteSnapshot(ctx, testSandbox, snap.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := p.Destroy(ctx, "copy"); err != nil {
		t.Fatalf("destroy the restored sandbox: %v", err)
	}
	if known, err := p.restoredLabels(); err != nil || len(known) != 0 {
		t.Fatalf("known = %v err = %v, want the record dropped", known, err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.snaps) != 1 || f.snaps["theirs"] == nil {
		t.Fatalf("snapshots left = %v, want only the user's", f.snaps)
	}
}
