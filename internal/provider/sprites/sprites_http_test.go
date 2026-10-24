package sprites_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/sbox/internal/provider/sprites"
	"github.com/perylemke/sbox/internal/sandbox"
)

func lifecycleTestHandler(page *int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sprites":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"name": "created-one"})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sprites":
			writeListPage(w, page)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/sprites/alpha":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}
}

func writeListPage(w http.ResponseWriter, page *int) {
	*page++
	if *page == 1 {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sprites": []map[string]string{
				{"name": "alpha", "status": "cold", "organization": "org", "id": "1"},
			},
			"has_more":                true,
			"next_continuation_token": "next",
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"sprites": []map[string]string{
			{"name": "beta", "status": "warm", "organization": "org", "id": "2"},
		},
		"has_more": false,
	})
}

func TestHTTPLifecycle(t *testing.T) {
	const token = "httptest-token-value" // #nosec G101 -- test fixture
	page := 0

	srv := httptest.NewServer(lifecycleTestHandler(&page))
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

	id, err := p.Create(ctx, sandbox.Spec{Name: "created-one"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id != "created-one" {
		t.Fatalf("id = %q", id)
	}

	list, err := p.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
	if list[0].Status != "cold" || list[1].Status != "warm" {
		t.Fatalf("statuses = %q, %q", list[0].Status, list[1].Status)
	}

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
