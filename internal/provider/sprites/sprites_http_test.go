package sprites_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/sbox/internal/provider/sprites"
	"github.com/perylemke/sbox/internal/sandbox"
)

func lifecycleTestHandler(page, createdGets *int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sprites":
			handleLifecycleCreate(w, r)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sprites/"+lifecycleSpriteName:
			handleLifecycleGetCreated(w, createdGets)
		case r.Method == http.MethodHead && strings.HasSuffix(r.URL.Path, "/exec"):
			writeSpriteVersion(w, r.URL.Path)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sprites":
			writeListPage(w, page)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/sprites/alpha":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}
}

func handleLifecycleCreate(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	var req struct {
		Name   string   `json:"name"`
		Labels []string `json:"labels"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.Name != lifecycleSpriteName || len(req.Labels) != 1 || req.Labels[0] != lifecycleSboxLabel {
		http.Error(w, "unexpected create payload", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{spriteJSONFieldName: lifecycleSpriteName})
}

func handleLifecycleGetCreated(w http.ResponseWriter, createdGets *int) {
	*createdGets++
	status := spriteStatusCold
	if *createdGets >= 2 {
		status = spriteStatusWarm
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		spriteJSONFieldName: lifecycleSpriteName,
		"status":            status,
		"organization":      "org",
		"id":                "99",
	})
}

func writeSpriteVersion(w http.ResponseWriter, path string) {
	switch {
	case strings.Contains(path, "/alpha/"):
		w.Header().Set("Sprite-Version", "0.0.4")
	case strings.Contains(path, "/beta/"):
		w.Header().Set("Sprite-Version", "0.0.5")
	}
	w.WriteHeader(http.StatusOK)
}

func writeListPage(w http.ResponseWriter, page *int) {
	*page++
	if *page == 1 {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sprites": []map[string]any{
				{spriteJSONFieldName: "alpha", "status": spriteStatusCold, "organization": "org", "id": "1", "labels": []string{"team=a"}},
			},
			"has_more":                true,
			"next_continuation_token": "next",
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"sprites": []map[string]string{
			{spriteJSONFieldName: "beta", "status": spriteStatusWarm, "organization": "org", "id": "2"},
		},
		"has_more": false,
	})
}

func assertLifecycleList(t *testing.T, list []sandbox.Info) {
	t.Helper()
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
	if list[0].Status != spriteStatusCold || list[1].Status != spriteStatusWarm {
		t.Fatalf("statuses = %q, %q", list[0].Status, list[1].Status)
	}
	if list[0].Version != "0.0.4" || list[1].Version != "0.0.5" {
		t.Fatalf("version = %q, %q", list[0].Version, list[1].Version)
	}
	if len(list[0].Labels) != 1 || list[0].Labels[0] != "team=a" {
		t.Fatalf("labels[0] = %v", list[0].Labels)
	}
}

func TestHTTPLifecycle(t *testing.T) {
	const token = "httptest-token-value" // #nosec G101 -- test fixture
	page := 0
	createdGets := 0

	srv := httptest.NewServer(lifecycleTestHandler(&page, &createdGets))
	t.Cleanup(srv.Close)

	p := sprites.New()
	if err := p.ConfigureWithOptions(token,
		spritesdk.WithBaseURL(srv.URL),
		spritesdk.WithDisableControl(),
	); err != nil {
		t.Fatalf("configure: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	id, err := p.Create(ctx, sandbox.Spec{Name: lifecycleSpriteName, Labels: []string{lifecycleSboxLabel}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id != lifecycleSpriteName {
		t.Fatalf("id = %q", id)
	}
	if createdGets < 2 {
		t.Fatalf("createdGets = %d, want at least 2 warm polls", createdGets)
	}

	list, err := p.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	assertLifecycleList(t, list)

	if err := p.Destroy(ctx, "alpha"); err != nil {
		t.Fatalf("destroy: %v", err)
	}
}

func TestHTTPErrorRedactsToken(t *testing.T) {
	const token = "secret-in-error-body" // #nosec G101 -- test fixture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "failed with "+token, http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	p := sprites.New()
	if err := p.ConfigureWithOptions(token,
		spritesdk.WithBaseURL(srv.URL),
		spritesdk.WithDisableControl(),
	); err != nil {
		t.Fatalf("configure: %v", err)
	}

	_, err := p.Create(context.Background(), sandbox.Spec{Name: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("error leaked token: %v", err)
	}
}
