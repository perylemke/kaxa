package daytona

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testKey = "dtn-secret-key"

func newTestClient(t *testing.T, h http.HandlerFunc) *restClient {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := newRESTClientAt(srv.URL, testKey)
	if err != nil {
		t.Fatalf("newRESTClientAt: %v", err)
	}
	return c
}

func TestCheckEndpoint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		url string
		ok  bool
	}{
		{"https://api.daytona.com", true},
		{"http://127.0.0.1:8080", true},
		{"http://localhost:1", true},
		{"http://api.daytona.com", false},
		{"https://user:pw@api.daytona.com", false},
		{"ftp://example.com", false},
		{"not a url", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			t.Parallel()
			_, err := checkEndpoint(tt.url)
			if (err == nil) != tt.ok {
				t.Fatalf("checkEndpoint(%q) err = %v, want ok=%v", tt.url, err, tt.ok)
			}
		})
	}
}

func TestCreateSendsManagedLabelsAndAuth(t *testing.T) {
	t.Parallel()
	var got map[string]any
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testKey {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Method != http.MethodPost || r.URL.Path != "/sandboxes" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": sandboxID, "name": boxName, "status": "queued",
			labelsKey: got[labelsKey],
		})
	})
	rec, err := c.Create(t.Context(), CreateParams{Name: boxName, Labels: []string{"sync"}, Template: "my-img", APIPort: 8000})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	expect(t, "persistent", got["persistent"], true)
	expect(t, "image", got["image"], "my-img")
	labels, _ := got[labelsKey].(map[string]any)
	expect(t, labelsKey, labels, map[string]any{
		labelManaged: labelTrue, labelName: boxName, labelLabels: "sync", labelAPIPort: "8000", labelTemplate: "my-img",
	})
	expect(t, "record", []any{rec.ID, rec.Name, rec.APIPort, rec.Template, rec.Labels}, []any{sandboxID, boxName, 8000, "my-img", []string{"sync"}})
}

func TestListPagesAndKeepsOnlyManaged(t *testing.T) {
	t.Parallel()
	managed := func(id, name, status string) map[string]any {
		return map[string]any{"id": id, "status": status, labelsKey: map[string]string{labelManaged: labelTrue, labelName: name}}
	}
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cursor") == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items":       []any{managed("1", "a", "running"), map[string]any{"id": "x", labelsKey: map[string]string{"team": otherName}}},
				"next_cursor": "c2",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{managed("2", "b", "stopped"), managed("3", "c", "deleted")}})
	})
	records, err := c.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 2 || records[0].Name != "a" || records[1].Name != "b" {
		t.Fatalf("records = %+v", records)
	}
}

func TestFindReturnsNewestAndNotFound(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		l := map[string]string{labelManaged: labelTrue, labelName: boxName}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{
			map[string]any{"id": "old", labelsKey: l, "created_at": "2026-01-01T00:00:00Z"},
			map[string]any{"id": newName, labelsKey: l, "created_at": "2026-02-01T00:00:00Z"},
		}})
	})
	rec, err := c.Find(t.Context(), boxName)
	if err != nil || rec.ID != newName {
		t.Fatalf("Find = %+v, %v", rec, err)
	}
	if _, err := c.Find(t.Context(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find missing err = %v", err)
	}
}

func TestAPIErrorCarriesMessageNotCredentials(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"no access"}`))
	})
	_, err := c.Get(t.Context(), sandboxID)
	api, ok := errors.AsType[*APIError](err)
	if !ok || api.Status != http.StatusForbidden || api.Message != "no access" {
		t.Fatalf("err = %v", err)
	}
	if userErr(err, boxName, testKey) == nil {
		t.Fatal("userErr returned nil")
	}
}

func TestGetMapsNotFound(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	if _, err := c.Get(t.Context(), "gone"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get err = %v", err)
	}
	if err := c.Delete(t.Context(), "gone"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete err = %v", err)
	}
}

// portAndImageHandler serves the port and image endpoints that TestSetPortAndImages calls.
func portAndImageHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/sandboxes/sb-1/ports/8000":
			var body map[string]bool
			_ = json.NewDecoder(r.Body).Decode(&body)
			if !body["public"] {
				t.Errorf("body = %v", body)
			}
			_, _ = w.Write([]byte(`{"port":8000,"public":true,"url":"https://8000-sb-1.proxy.daytona.works"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/sandboxes/sb-1/images":
			_, _ = w.Write([]byte(`{"id":"img_1","name":"kaxa-box-1","status":"pending"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/images":
			_, _ = w.Write([]byte(`{"items":[{"id":"img_1","name":"kaxa-box-1"},{"id":"img_2","name":"other"}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/images/img_1":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func TestSetPortAndImages(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, portAndImageHandler(t))
	ep, err := c.SetPort(t.Context(), sandboxID, 8000, true)
	if err != nil {
		t.Fatalf("SetPort: %v", err)
	}
	expect(t, "SetPort", []any{ep.Public, ep.URL != ""}, []any{true, true})
	img, err := c.CaptureImage(t.Context(), sandboxID, "kaxa-box-1")
	if err != nil {
		t.Fatalf("CaptureImage: %v", err)
	}
	expect(t, "CaptureImage id", img.ID, "img_1")
	imgs, err := c.ListImages(t.Context(), "kaxa-")
	if err != nil {
		t.Fatalf("ListImages: %v", err)
	}
	expect(t, "ListImages count", len(imgs), 1)
	if _, err := c.GetImage(t.Context(), "missing"); !errors.Is(err, ErrImageNotFound) {
		t.Fatalf("GetImage err = %v", err)
	}
	if err := c.DeleteImage(t.Context(), "img_1"); err != nil {
		t.Fatalf("DeleteImage: %v", err)
	}
}
