package docker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

const (
	snapUID      = "abc"
	snapResource = "snapshots/abc"
	testCopy     = "copy"
	snapPath     = "/v1/snapshots/abc"
	snapJSONRaw  = `{"uid":"abc","name":"snapshots/abc","displayName":"kaxa-box-` + testSnapshotAt + `",` +
		`"description":"before","status":"ready","createdAt":"2026-01-02T03:04:05Z","etag":"\"s1\""}`
)

func TestSnapshotDisplayNameRoundTrips(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	name := snapshotDisplayName("my-box", at)
	if name != "kaxa-my-box-"+testSnapshotAt {
		t.Fatalf("display name = %q", name)
	}
	got, ok := snapshotSandbox(name)
	if !ok || got != "my-box" {
		t.Fatalf("sandbox = %q, %v", got, ok)
	}
}

func TestSnapshotSandboxRejectsOtherSnapshots(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"", "backup", "kaxa-", "kaxa-box", "kaxa-box-yesterday", "kaxa--" + testSnapshotAt,
		"kaxa-Box-" + testSnapshotAt, "kaxa-box-20261340250000",
	} {
		if got, ok := snapshotSandbox(name); ok {
			t.Errorf("snapshotSandbox(%q) = %q, want not kaxa's", name, got)
		}
	}
}

func TestCreateSnapshotSendsDiskCapture(t *testing.T) {
	t.Parallel()
	c, srv := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"uid":"abc","name":"snapshots/abc","displayName":"n","status":"creating","createdAt":"2026-01-02T03:04:05Z"}`))
	})
	got, err := c.CreateSnapshot(context.Background(), Record{Resource: testRes}, "n", "before")
	if err != nil || got.ID != snapUID || got.Status != snapshotCreating || got.Resource != snapResource {
		t.Fatalf("snapshot = %+v, err = %v", got, err)
	}
	req := srv.last(t)
	if req.Method != http.MethodPost || req.URL.Path != "/v1/sandboxes/abc/snapshots" || len(req.Header.Get("Idempotency-Key")) != 32 {
		t.Fatalf("request = %s %s key=%q", req.Method, req.URL.Path, req.Header.Get("Idempotency-Key"))
	}
	raw, _ := json.Marshal(srv.body)
	if string(raw) != `{"captureMode":"disk","description":"before","displayName":"n"}` {
		t.Fatalf("body = %s", raw)
	}
}

func TestCreateSnapshotMapsConflict(t *testing.T) {
	t.Parallel()
	c, _ := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"alreadyExists"}`))
	})
	_, err := c.CreateSnapshot(context.Background(), Record{Resource: testRes}, "n", "")
	if !errors.Is(err, ErrSnapshotExists) {
		t.Fatalf("err = %v, want ErrSnapshotExists", err)
	}
}

func TestListSnapshotsFollowsPages(t *testing.T) {
	t.Parallel()
	c, srv := newAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageToken") == "" {
			_, _ = w.Write([]byte(`{"snapshots":[{"uid":"a","name":"snapshots/a","status":"ready"}],"nextPageToken":"p2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"snapshots":[{"name":"snapshots/b","status":"creating"}]}`))
	})
	got, err := c.ListSnapshots(context.Background())
	if err != nil || len(got) != 2 {
		t.Fatalf("snapshots = %+v, err = %v", got, err)
	}
	if got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("ids = %q, %q (the ID falls back to the resource name)", got[0].ID, got[1].ID)
	}
	if req := srv.last(t); req.URL.Path != "/v1/snapshots" || req.URL.Query().Get("pageSize") != pageSize {
		t.Fatalf("last request = %s", req.URL)
	}
}

func TestGetSnapshotReadsFields(t *testing.T) {
	t.Parallel()
	c, srv := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"hdr"`)
		_, _ = w.Write([]byte(snapJSONRaw))
	})
	got, err := c.GetSnapshot(context.Background(), snapResource)
	if err != nil {
		t.Fatal(err)
	}
	want := SnapshotRecord{
		ID: snapUID, Resource: snapResource, DisplayName: "kaxa-box-" + testSnapshotAt,
		Description: "before", Status: snapshotReady, Etag: `"hdr"`,
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	if got != want {
		t.Fatalf("snapshot = %+v, want %+v", got, want)
	}
	if got.Sandbox() != testSandbox {
		t.Fatalf("Sandbox() = %q", got.Sandbox())
	}
	if req := srv.last(t); req.URL.Path != snapPath {
		t.Fatalf("path = %s", req.URL.Path)
	}
}

func TestGetSnapshotReadsFailure(t *testing.T) {
	t.Parallel()
	c, _ := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"uid":"abc","name":"snapshots/abc","status":"failed","failure":{"code":"internal","message":"disk full"}}`))
	})
	got, err := c.GetSnapshot(context.Background(), snapResource)
	if err != nil || got.Failure != "disk full" {
		t.Fatalf("snapshot = %+v, err = %v", got, err)
	}
}

func TestGetSnapshotNotFound(t *testing.T) {
	t.Parallel()
	c, _ := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"notFound"}`))
	})
	if _, err := c.GetSnapshot(context.Background(), snapResource); !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("err = %v, want ErrSnapshotNotFound", err)
	}
}

func TestDeleteSnapshotSendsEtag(t *testing.T) {
	t.Parallel()
	c, srv := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	err := c.DeleteSnapshot(context.Background(), SnapshotRecord{Resource: snapResource, Etag: snapEtag})
	if err != nil {
		t.Fatal(err)
	}
	req := srv.last(t)
	if req.Method != http.MethodDelete || req.URL.Path != snapPath || req.Header.Get("If-Match") != snapEtag {
		t.Fatalf("request = %s %s if-match=%q", req.Method, req.URL.Path, req.Header.Get("If-Match"))
	}
}

func TestDeleteSnapshotNotFound(t *testing.T) {
	t.Parallel()
	c, _ := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"notFound"}`))
	})
	err := c.DeleteSnapshot(context.Background(), SnapshotRecord{Resource: snapResource})
	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("err = %v, want ErrSnapshotNotFound", err)
	}
}

func TestRestoreSnapshotCreatesANamedSandbox(t *testing.T) {
	t.Parallel()
	c, srv := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"name":"sandboxes/new","displayName":"copy","core":{"status":"starting","labels":{"` + managedLabel + `":"true"}}}`))
	})
	got, err := c.RestoreSnapshot(context.Background(), SnapshotRecord{Resource: snapResource}, testCopy)
	if err != nil || got.Name != testCopy || got.Resource != "sandboxes/new" || !got.Managed() {
		t.Fatalf("record = %+v, err = %v", got, err)
	}
	req := srv.last(t)
	if req.Method != http.MethodPost || req.URL.Path != snapPath+"/restore" || len(req.Header.Get("Idempotency-Key")) != 32 {
		t.Fatalf("request = %s %s key=%q", req.Method, req.URL.Path, req.Header.Get("Idempotency-Key"))
	}
	raw, _ := json.Marshal(srv.body)
	if string(raw) != `{"displayName":"copy"}` {
		t.Fatalf("body = %s", raw)
	}
}

func TestRestoreSnapshotMapsErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"name taken", http.StatusConflict, `{"code":"alreadyExists"}`, ErrAlreadyExists},
		{"snapshot gone", http.StatusNotFound, `{"code":"notFound"}`, ErrSnapshotNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, _ := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})
			_, err := c.RestoreSnapshot(context.Background(), SnapshotRecord{Resource: snapResource}, testCopy)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}
