package daytona

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestSDKClientReads(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	c := f.client()
	ctx := context.Background()

	rec, err := c.Get(ctx, sandboxID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	expect(t, "name", rec.Name, boxName)
	expect(t, keyStatus, rec.Status, statusRunning)
	expect(t, "cpu", rec.CPU, int32(2))

	found, err := c.Find(ctx, boxName)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	expect(t, "found id", found.ID, sandboxID)
	if _, err := c.Find(ctx, "wrong"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find other name = %v", err)
	}

	list, err := c.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %v, %v", list, err)
	}
}

func TestSDKClientActions(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	c := f.client()
	ctx := context.Background()

	if err := c.Start(ctx, sandboxID); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := c.Resume(ctx, sandboxID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	f.mu.Lock()
	f.state = statusStopped
	f.mu.Unlock()
	if err := c.Stop(ctx, sandboxID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	f.mu.Lock()
	f.state = "destroyed"
	f.mu.Unlock()
	if err := c.Delete(ctx, sandboxID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	for _, line := range []string{"/start", "/stop", "DELETE /api/sandbox/"} {
		if !f.seen(line) {
			t.Errorf("no request for %s", line)
		}
	}
}

func TestSDKClientPorts(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	c := f.client()
	ctx := context.Background()

	got, err := c.GetPort(ctx, sandboxID, 80)
	if err != nil {
		t.Fatalf("GetPort: %v", err)
	}
	expect(t, "url", got.URL, "https://80-box.example")
	expect(t, "port", got.Port, 80)

	if _, err := c.SetPort(ctx, sandboxID, 80, true); err != nil {
		t.Fatalf("SetPort public: %v", err)
	}
	if !f.seen("/public/true") {
		t.Error("sandbox was not made public")
	}
	if _, err := c.SetPort(ctx, sandboxID, 80, false); err != nil {
		t.Fatalf("SetPort private: %v", err)
	}
}

func TestSDKClientCreate(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	f.mu.Lock()
	f.labels[labelLabels] = "a,b"
	f.labels[labelAPIPort] = "8080"
	f.mu.Unlock()
	c := f.client()

	rec, err := c.Create(context.Background(), CreateParams{
		Name: boxName, Labels: []string{"a", "b"}, Template: "tpl", APIPort: 8080, NetworkBlockAll: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	expect(t, "labels", rec.Labels, []string{"a", "b"})
	expect(t, "api port", rec.APIPort, 8080)
	body := f.body("POST /api/sandbox")
	for _, want := range []string{labelManaged, `"tpl"`, "8080", `"kaxa.name":"box"`} {
		if !strings.Contains(body, want) {
			t.Errorf("create body %s lacks %s", body, want)
		}
	}
}

func TestSDKClientCreateErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		status   int
		labels   map[string]string
		wantIs   error
		wantText string
	}{
		{"name taken", http.StatusConflict, nil, ErrAlreadyExists, ""},
		{"server error", http.StatusInternalServerError, nil, nil, "500"},
		{"not managed", http.StatusOK, map[string]string{"x": "y"}, nil, "no managed sandbox"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFakeServer(t)
			if tt.labels != nil {
				f.labels = tt.labels
			}
			if tt.status != http.StatusOK {
				f.override = func(r *http.Request) (int, any, bool) {
					return tt.status, errBody(tt.status, "nope"), r.Method == http.MethodPost
				}
			}
			_, err := f.client().Create(context.Background(), CreateParams{Name: boxName})
			if err == nil {
				t.Fatal("Create succeeded")
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Fatalf("err = %v, want %v", err, tt.wantIs)
			}
			if tt.wantText != "" && !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("err = %v, want text %q", err, tt.wantText)
			}
		})
	}
}

func TestSDKClientMissingSandbox(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	f.override = func(*http.Request) (int, any, bool) {
		return http.StatusNotFound, errBody(404, "gone"), true
	}
	c := f.client()
	ctx := context.Background()

	calls := map[string]func() error{
		"Get":      func() error { _, err := c.Get(ctx, sandboxID); return err },
		"Find":     func() error { _, err := c.Find(ctx, boxName); return err },
		"Delete":   func() error { return c.Delete(ctx, sandboxID) },
		"Start":    func() error { return c.Start(ctx, sandboxID) },
		"Resume":   func() error { return c.Resume(ctx, sandboxID) },
		"Stop":     func() error { return c.Stop(ctx, sandboxID) },
		"SetPort":  func() error { _, err := c.SetPort(ctx, sandboxID, 80, true); return err },
		"GetPort":  func() error { _, err := c.GetPort(ctx, sandboxID, 80); return err },
		"Capture":  func() error { _, err := c.CaptureImage(ctx, sandboxID, "n"); return err },
		"GetImage": func() error { _, err := c.GetImage(ctx, "n"); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := call()
			want := ErrNotFound
			if name == "GetImage" {
				want = ErrImageNotFound
			}
			if !errors.Is(err, want) {
				t.Fatalf("err = %v, want %v", err, want)
			}
		})
	}
	if err := c.DeleteImage(ctx, "n"); !errors.Is(err, ErrImageNotFound) {
		t.Fatalf("DeleteImage = %v", err)
	}
}

func TestSDKClientActionFailures(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	f.override = func(r *http.Request) (int, any, bool) {
		if strings.HasPrefix(r.URL.Path, "/api/sandbox/") && r.Method != http.MethodGet || strings.Contains(r.URL.Path, "preview") {
			return http.StatusInternalServerError, errBody(500, msgBoom), true
		}
		return 0, nil, false
	}
	c := f.client()
	ctx := context.Background()
	var api *APIError
	for name, err := range map[string]error{
		"Start":   c.Start(ctx, sandboxID),
		"Stop":    c.Stop(ctx, sandboxID),
		"Delete":  c.Delete(ctx, sandboxID),
		"SetPort": func() error { _, err := c.SetPort(ctx, sandboxID, 80, true); return err }(),
		"GetPort": func() error { _, err := c.GetPort(ctx, sandboxID, 80); return err }(),
		"Capture": func() error { _, err := c.CaptureImage(ctx, sandboxID, "n"); return err }(),
	} {
		if !errors.As(err, &api) || api.Status != 500 {
			t.Errorf("%s: err = %v, want API 500", name, err)
		}
	}
}

func TestSDKClientImages(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	c := f.client()
	ctx := context.Background()

	imgs, err := c.ListImages(ctx, "kaxa-")
	if err != nil || len(imgs) != 1 || imgs[0].Name != "kaxa-a" {
		t.Fatalf("ListImages = %v, %v", imgs, err)
	}
	rows, err := c.ListSnapshotCatalog(ctx)
	if err != nil || len(rows) != 2 {
		t.Fatalf("ListSnapshotCatalog = %v, %v", rows, err)
	}
	expect(t, "catalog state", rows[0].State, snapshotStateActive)
	img, err := c.GetImage(ctx, "kaxa-a")
	if err != nil {
		t.Fatalf("GetImage: %v", err)
	}
	expect(t, "image name", img.Name, "kaxa-a")
	if err := c.DeleteImage(ctx, "kaxa-a"); err != nil {
		t.Fatalf("DeleteImage: %v", err)
	}
}

func TestSDKClientCaptureImage(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	img, err := f.client().CaptureImage(context.Background(), sandboxID, "kaxa-a")
	if err != nil {
		t.Fatalf("CaptureImage: %v", err)
	}
	expect(t, "image", img.Name, "kaxa-a")
}

func TestSDKClientListFailures(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	f.override = func(*http.Request) (int, any, bool) {
		return http.StatusForbidden, errBody(403, "no"), true
	}
	c := f.client()
	ctx := context.Background()
	if _, err := c.List(ctx); err == nil {
		t.Error("List succeeded")
	}
	if _, err := c.ListImages(ctx, ""); err == nil {
		t.Error("ListImages succeeded")
	}
	if _, err := c.ListSnapshotCatalog(ctx); err == nil {
		t.Error("ListSnapshotCatalog succeeded")
	}
}

func TestSDKClientHandleIsCached(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	c := f.client()
	ctx := context.Background()
	if _, err := c.handle(ctx, sandboxID); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.requests = nil
	f.mu.Unlock()
	if _, err := c.handle(ctx, boxName); err != nil {
		t.Fatal(err)
	}
	if f.seen("GET") {
		t.Error("cached handle read the sandbox again")
	}
}

func TestNewSDKClientRejectsEmptyKey(t *testing.T) {
	t.Setenv("DAYTONA_API_KEY", "")
	t.Setenv("DAYTONA_JWT_TOKEN", "")
	if _, err := newSDKClient(""); err == nil {
		t.Fatal("empty key accepted")
	}
}
