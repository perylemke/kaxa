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

func TestHTTPInspectRestartUpgrade(t *testing.T) {
	const token = "actions-test-token" // #nosec G101 -- test fixture
	const spriteName = "demo"

	var restartCalls, upgradeCalls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sprites/"+spriteName:
			_ = json.NewEncoder(w).Encode(map[string]any{
				spriteJSONFieldName: spriteName,
				"status":            spriteStatusWarm,
				"organization":      "acme",
				"id":                "uuid-1",
				"url":               "https://demo.sprites.dev",
				"url_settings": map[string]string{
					"auth":           "sprite",
					"private_access": "admins",
				},
				"labels":     []string{"sbox"},
				"created_at": time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
				"updated_at": time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sprites/"+spriteName+"/restart":
			restartCalls++
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"message":"Restart requested; the machine will reboot shortly."}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sprites/"+spriteName+"/upgrade":
			upgradeCalls++
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	p := sprites.New()
	if err := p.ConfigureWithOptions(token, srv.URL, spritesdk.WithDisableControl()); err != nil {
		t.Fatalf("configure: %v", err)
	}

	ctx := context.Background()
	id := sandbox.ID(spriteName)

	detail, err := p.Inspect(ctx, id)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if detail.SpriteUUID != "uuid-1" || detail.Organization != "acme" {
		t.Fatalf("detail = %+v", detail)
	}
	if detail.URL != "https://demo.sprites.dev" {
		t.Fatalf("url = %q", detail.URL)
	}
	if detail.Auth != "sprite · admins" {
		t.Fatalf("auth = %q", detail.Auth)
	}
	if len(detail.Labels) != 1 || detail.Labels[0] != lifecycleSboxLabel {
		t.Fatalf("labels = %v", detail.Labels)
	}

	if err := p.Restart(ctx, id); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if err := p.Upgrade(ctx, id); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if restartCalls != 1 || upgradeCalls != 1 {
		t.Fatalf("restartCalls=%d upgradeCalls=%d", restartCalls, upgradeCalls)
	}
}
