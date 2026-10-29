package sprites_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/kaxa/internal/provider/sprites"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

const snapshotToken = "snapshot-test-token" // #nosec G101 -- test fixture

// checkpointServer is an in-memory Sprites checkpoint API.
type checkpointServer struct {
	mu          sync.Mutex
	checkpoints []map[string]any
	restored    string
	deleted     []string
	streamError string
	// createExtra is how many checkpoints one create adds besides its own, to
	// mimic a second create landing in the same window. -1 adds none at all.
	createExtra int
	deleteCode  int
	deleteBody  string
	badAuth     bool
}

func (s *checkpointServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer "+snapshotToken {
		s.badAuth = true
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	base := "/v1/sprites/" + spriteName
	switch {
	case r.Method == http.MethodPost && r.URL.Path == base+"/checkpoint":
		s.create(w, r)
	case r.Method == http.MethodGet && r.URL.Path == base+"/checkpoints":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(append([]map[string]any{}, s.checkpoints...))
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/restore"):
		s.restored = strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, base+"/checkpoints/"), "/restore")
		s.stream(w)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, base+"/checkpoints/"):
		s.remove(w, r, base)
	default:
		http.NotFound(w, r)
	}
}

func (s *checkpointServer) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Comment string `json:"comment"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if s.streamError == "" {
		n := 1 + max(s.createExtra, 0)
		if s.createExtra < 0 {
			n = 0
		}
		for range n {
			s.checkpoints = append(s.checkpoints, map[string]any{
				"id":          fmt.Sprintf("v%d", len(s.checkpoints)+1),
				"create_time": time.Date(2026, 1, 1, 0, len(s.checkpoints), 0, 0, time.UTC).Format(time.RFC3339),
				"comment":     body.Comment,
			})
		}
	}
	s.stream(w)
}

func (s *checkpointServer) stream(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	_, _ = fmt.Fprintln(w, `{"type":"info","data":"working"}`)
	if s.streamError != "" {
		_, _ = fmt.Fprintf(w, "{\"type\":\"error\",\"error\":%q}\n", s.streamError)
		return
	}
	_, _ = fmt.Fprintln(w, `{"type":"info","data":"done"}`)
}

func (s *checkpointServer) remove(w http.ResponseWriter, r *http.Request, base string) {
	if s.deleteCode != 0 {
		w.WriteHeader(s.deleteCode)
		_, _ = w.Write([]byte(s.deleteBody))
		return
	}
	s.deleted = append(s.deleted, strings.TrimPrefix(r.URL.Path, base+"/checkpoints/"))
	w.WriteHeader(http.StatusNoContent)
}

func newSnapshotProvider(t *testing.T, srv *checkpointServer) *sprites.Provider {
	t.Helper()
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	p := sprites.New()
	if err := p.ConfigureWithOptions(snapshotToken, ts.URL, spritesdk.WithDisableControl()); err != nil {
		t.Fatalf("configure: %v", err)
	}
	return p
}

func TestSnapshotRoundTrip(t *testing.T) {
	t.Parallel()

	srv := &checkpointServer{}
	p := newSnapshotProvider(t, srv)
	ctx := context.Background()
	id := sandbox.ID(spriteName)

	if p.RestoreMode() != sandbox.RestoreInPlace {
		t.Fatalf("restore mode = %q", p.RestoreMode())
	}
	first, err := p.Snapshot(ctx, id, "first")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	second, err := p.Snapshot(ctx, id, "second")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if first.ID != "v1" || second.ID != "v2" || second.Comment != "second" || second.Provider != spritesName || second.Sandbox != id {
		t.Fatalf("snapshots = %+v, %+v", first, second)
	}
	list, err := p.ListSnapshots(ctx, id)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].ID != "v2" || list[1].ID != "v1" {
		t.Fatalf("list not newest first: %+v", list)
	}
	restoreAndDeleteCheckpoint(t, p, srv)
}

func restoreAndDeleteCheckpoint(t *testing.T, p *sprites.Provider, srv *checkpointServer) {
	t.Helper()
	ctx := context.Background()
	id := sandbox.ID(spriteName)
	got, err := p.Restore(ctx, sandbox.RestoreRequest{Sandbox: id, Snapshot: "v1"})
	if err != nil || got != id {
		t.Fatalf("restore = %q, %v", got, err)
	}
	if srv.restored != "v1" {
		t.Fatalf("restored = %q", srv.restored)
	}
	if err := p.DeleteSnapshot(ctx, id, "v1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(srv.deleted) != 1 || srv.deleted[0] != "v1" {
		t.Fatalf("deleted = %v", srv.deleted)
	}
	if srv.badAuth {
		t.Fatal("a request carried the wrong token")
	}
}

func TestSnapshotStreamError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(p *sprites.Provider) error
	}{
		{"create", func(p *sprites.Provider) error {
			_, err := p.Snapshot(context.Background(), spriteName, "")
			return err
		}},
		{"restore", func(p *sprites.Provider) error {
			_, err := p.Restore(context.Background(), sandbox.RestoreRequest{Sandbox: spriteName, Snapshot: "v1"})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newSnapshotProvider(t, &checkpointServer{streamError: "disk full"})
			err := tt.run(p)
			if err == nil || !strings.Contains(err.Error(), "disk full") {
				t.Fatalf("err = %v, want stream error", err)
			}
		})
	}
}

func TestDeleteSnapshotStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		code     int
		body     string
		wantErr  bool
		wantText string
	}{
		{"no content", http.StatusNoContent, "", false, ""},
		{"ok", http.StatusOK, "", false, ""},
		{"not found", http.StatusNotFound, "nope", true, "not found"},
		{"server error redacts token", http.StatusInternalServerError, "boom " + snapshotToken, true, "<redacted>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newSnapshotProvider(t, &checkpointServer{deleteCode: tt.code, deleteBody: tt.body})
			err := p.DeleteSnapshot(context.Background(), spriteName, "v1")
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				return
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("err = %q, want %q", err, tt.wantText)
			}
			if strings.Contains(err.Error(), snapshotToken) {
				t.Fatalf("error leaks the token: %q", err)
			}
		})
	}
}

func TestSnapshotRejectsBadInput(t *testing.T) {
	t.Parallel()

	p := newSnapshotProvider(t, &checkpointServer{})
	ctx := context.Background()

	if _, err := p.Snapshot(ctx, "Bad/Name", ""); err == nil {
		t.Error("snapshot accepted a bad sandbox name")
	}
	if _, err := p.Snapshot(ctx, spriteName, "a\nb"); err == nil {
		t.Error("snapshot accepted a comment with a control character")
	}
	if _, err := p.Restore(ctx, sandbox.RestoreRequest{Sandbox: spriteName, Snapshot: "../x"}); err == nil {
		t.Error("restore accepted a path in the snapshot id")
	}
	if _, err := p.Restore(ctx, sandbox.RestoreRequest{Sandbox: spriteName, Snapshot: "v1", Target: "other"}); err == nil {
		t.Error("restore accepted a target")
	}
	if err := p.DeleteSnapshot(ctx, spriteName, "a/b"); err == nil {
		t.Error("delete accepted a path in the snapshot id")
	}
}

// The API lists the live state of the sprite as a checkpoint named "Current".
// It is not a snapshot, so it must never reach the user.
func TestSnapshotHidesCurrent(t *testing.T) {
	t.Parallel()

	srv := &checkpointServer{checkpoints: []map[string]any{{
		"id":          "Current",
		"create_time": time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"comment":     "",
	}}}
	p := newSnapshotProvider(t, srv)
	ctx := context.Background()

	snap, err := p.Snapshot(ctx, spriteName, "first")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snap.ID == "Current" {
		t.Fatalf("snapshot = %+v, want the new checkpoint", snap)
	}
	list, err := p.ListSnapshots(ctx, spriteName)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].ID != snap.ID {
		t.Fatalf("list = %+v, want only %q", list, snap.ID)
	}
	if _, err := p.Restore(ctx, sandbox.RestoreRequest{Sandbox: spriteName, Snapshot: "Current"}); err == nil {
		t.Error("restore accepted the live state as a snapshot")
	}
	if err := p.DeleteSnapshot(ctx, spriteName, "Current"); err == nil {
		t.Error("delete accepted the live state as a snapshot")
	}
	if srv.restored != "" || len(srv.deleted) != 0 {
		t.Errorf("a request reached the API: restored=%q deleted=%v", srv.restored, srv.deleted)
	}
}

// The new checkpoint is found by diffing two lists, so it is only trusted when
// exactly one checkpoint appeared. Anything else could be another create.
func TestSnapshotRefusesAnAmbiguousCheckpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		extra int
	}{
		{"another checkpoint appeared", 1},
		{"no checkpoint appeared", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := newSnapshotProvider(t, &checkpointServer{createExtra: tt.extra})
			_, err := p.Snapshot(context.Background(), spriteName, "x")
			ue, ok := usererr.As(err)
			if !ok {
				t.Fatalf("err = %v, want a user error", err)
			}
			if want := "kaxa snapshot list " + spriteName; !strings.Contains(ue.Hint, want) {
				t.Fatalf("hint = %q, want %q", ue.Hint, want)
			}
		})
	}
}

func TestSnapshotNotConfigured(t *testing.T) {
	t.Parallel()

	_, err := sprites.New().ListSnapshots(context.Background(), spriteName)
	if !errors.Is(err, sandbox.ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

func TestSpritesIsSnapshotter(t *testing.T) {
	t.Parallel()

	caps := sandbox.CapabilitiesOf(sprites.New())
	if !caps.Snapshot || caps.SnapshotRestore != string(sandbox.RestoreInPlace) {
		t.Fatalf("capabilities = %+v", caps)
	}
}
