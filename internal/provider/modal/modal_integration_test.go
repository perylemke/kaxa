//go:build integration

package modal_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/provider/modal"
	"github.com/perylemke/kaxa/internal/sandbox"
)

// TestRealModalCRUD creates, lists, inspects, and destroys one sandbox on the
// Modal API. It is skipped unless KAXA_MODAL_TOKEN_ID and
// KAXA_MODAL_TOKEN_SECRET are set, and it bills a few seconds of compute.
func TestRealModalCRUD(t *testing.T) {
	id, secret := os.Getenv("KAXA_MODAL_TOKEN_ID"), os.Getenv("KAXA_MODAL_TOKEN_SECRET")
	if id == "" || secret == "" {
		t.Skip("KAXA_MODAL_TOKEN_ID and KAXA_MODAL_TOKEN_SECRET are not set")
	}

	p := modal.New()
	if err := p.ConfigureCredentials(id, secret); err != nil {
		t.Fatalf("configure: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	name := "kaxa-it-" + randomSuffix(t)
	sid, err := p.Create(ctx, sandbox.Spec{Name: name, Labels: []string{"kaxa", "integration"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		_ = p.Destroy(cleanup, sid)
	})

	if _, err := p.Create(ctx, sandbox.Spec{Name: name}); err == nil {
		t.Fatal("duplicate create succeeded")
	}

	infos, err := p.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !containsID(infos, sid) {
		t.Fatalf("list %+v does not contain %q", infos, sid)
	}

	detail, err := p.Inspect(ctx, sid)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if detail.ID != sid || detail.Status != "running" || len(detail.Labels) != 2 {
		t.Fatalf("detail = %+v", detail)
	}

	if err := p.Destroy(ctx, sid); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if _, err := p.Inspect(ctx, sid); err == nil {
		t.Fatal("inspect after destroy succeeded")
	}
	infos, err = p.List(ctx)
	if err != nil {
		t.Fatalf("list after destroy: %v", err)
	}
	if containsID(infos, sid) {
		t.Fatalf("list still contains %q", sid)
	}
}

func TestRealModalRejectsBadToken(t *testing.T) {
	if os.Getenv("KAXA_MODAL_TOKEN_ID") == "" {
		t.Skip("KAXA_MODAL_TOKEN_ID is not set; skipping network test")
	}
	p := modal.New()
	if err := p.ConfigureCredentials("ak-invalid", "as-invalid"); err != nil {
		t.Fatalf("configure: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, err := p.List(ctx); err == nil || errors.Is(err, modal.ErrMissingCredentials) {
		t.Fatalf("list with a bad token err = %v, want an API error", err)
	}
}

func containsID(infos []sandbox.Info, id sandbox.ID) bool {
	for _, i := range infos {
		if i.ID == id {
			return true
		}
	}
	return false
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return hex.EncodeToString(b)
}
