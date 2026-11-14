package blaxel

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	sdk "github.com/blaxel-ai/sdk-go"
	"github.com/blaxel-ai/sdk-go/option"
)

const (
	sdkSnap = `{"id":"snap-1","name":"snap-1","status":"ready","createdAt":"2026-01-02T03:04:05Z","workspace":"ws","source":{"name":"box","kind":"sandbox"}}`
	sdkBox  = `{"metadata":{"name":"box","createdAt":"2026-01-02T03:04:05Z","labels":{"kaxa.managed":"true"}},"spec":{"region":"us-pdx-1","runtime":{"image":"blaxel/base-image:latest"}},"status":"DEPLOYED"}`
)

func TestSDKSnapshotReads(t *testing.T) {
	t.Parallel()
	client := sdkClientFor(newSnapshotServer(t).URL)
	ctx := t.Context()

	row, err := client.CreateSnapshot(ctx, boxName)
	if err != nil || row.ID != "snap-1" || row.Sandbox != boxName {
		t.Fatalf("create = %+v, %v", row, err)
	}
	row, err = client.GetSnapshot(ctx, "snap-1")
	if err != nil || row.Status != snapReady {
		t.Fatalf("get = %+v, %v", row, err)
	}
	if _, err := client.GetSnapshot(ctx, "missing"); err == nil {
		t.Fatal("missing snapshot returned a row")
	}
	rows, err := client.ListSandboxSnapshots(ctx, boxName)
	if err != nil || len(rows) != 1 {
		t.Fatalf("sandbox list = %+v, %v", rows, err)
	}
	rows, err = client.ListWorkspaceSnapshots(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("workspace list = %+v, %v", rows, err)
	}
}

func TestSDKSnapshotWrites(t *testing.T) {
	t.Parallel()
	client := sdkClientFor(newSnapshotServer(t).URL)
	ctx := t.Context()
	if err := client.DeleteSnapshot(ctx, "snap-1"); err != nil {
		t.Fatal(err)
	}
	if err := client.RestoreSnapshot(ctx, boxName, "snap-1"); err != nil {
		t.Fatal(err)
	}
	rec, err := client.Unarchive(ctx, boxName)
	if err != nil || rec.Name != boxName || rec.Status != statusRunning {
		t.Fatalf("unarchive = %+v, %v", rec, err)
	}
	if _, err := client.Unarchive(ctx, "other"); err == nil || !strings.Contains(err.Error(), "unmanaged") {
		t.Fatalf("unmanaged err = %v", err)
	}
}

func TestSDKSnapshotCreateWithoutID(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"","name":"x","status":"ready","createdAt":"","workspace":"ws"}`))
	}))
	t.Cleanup(srv.Close)
	if _, err := sdkClientFor(srv.URL).CreateSnapshot(t.Context(), boxName); err == nil {
		t.Fatal("snapshot without an id was accepted")
	}
}

func newSnapshotServer(t *testing.T) *httptest.Server {
	t.Helper()
	var pages int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case http.MethodPost + " /snapshots", http.MethodGet + " /snapshots/snap-1":
			_, _ = w.Write([]byte(sdkSnap))
		case http.MethodGet + " /snapshots/missing":
			http.Error(w, `{"message":"missing"}`, http.StatusNotFound)
		case http.MethodGet + " /snapshots":
			writeSnapshotPage(w, &mu, &pages)
		case http.MethodDelete + " /snapshots/snap-1":
			w.WriteHeader(http.StatusNoContent)
		case http.MethodGet + " /sandboxes/box/snapshots":
			_, _ = w.Write([]byte("[" + sdkSnap + `,{"id":"","name":"skip","status":"ready","createdAt":"","workspace":"ws"}]`))
		case http.MethodPost + " /sandboxes/box/snapshots/snap-1/restore":
			_, _ = w.Write([]byte(`{"name":"box"}`))
		case http.MethodPost + " /sandboxes/box/unarchive":
			_, _ = w.Write([]byte(sdkBox))
		case http.MethodPost + " /sandboxes/other/unarchive":
			_, _ = w.Write([]byte(`{"metadata":{"name":"other"},"spec":{"region":"us-pdx-1","runtime":{"image":"img"}},"status":"DEPLOYED"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func writeSnapshotPage(w http.ResponseWriter, mu *sync.Mutex, pages *int) {
	mu.Lock()
	*pages++
	page := *pages
	mu.Unlock()
	if page == 1 {
		_, _ = w.Write([]byte(`{"data":[` + sdkSnap + `],"meta":{"hasMore":true,"nextCursor":"c2"}}`))
		return
	}
	_, _ = w.Write([]byte(`{"data":[],"meta":{"hasMore":false}}`))
}

func sdkClientFor(base string) *sdkClient {
	opts := []option.RequestOption{
		option.WithBaseURL(base),
		option.WithAPIKey("test-key"),
		option.WithWorkspace("ws"),
	}
	snaps := sdk.NewSnapshotService(opts...)
	sandboxSnaps := sdk.NewSandboxSnapshotService(opts...)
	return &sdkClient{
		apiKey: "test-key", workspace: "ws", opts: opts,
		snapshots: &snaps, sandboxSnapshots: &sandboxSnaps,
	}
}
