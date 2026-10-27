package config_test

import (
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/config"
	"github.com/perylemke/kaxa/internal/provider/modal"
	"github.com/perylemke/kaxa/internal/provider/sprites"
	"github.com/perylemke/kaxa/internal/registry"
)

func testRegistry(t *testing.T) *registry.Registry {
	t.Helper()
	reg := registry.New()
	if err := reg.Register(sprites.New()); err != nil {
		t.Fatalf("register sprites: %v", err)
	}
	if err := reg.Register(modal.New()); err != nil {
		t.Fatalf("register modal: %v", err)
	}
	return reg
}

func TestLoadFlagOverridesEnv(t *testing.T) {
	reg := testRegistry(t)
	t.Setenv("KAXA_DEFAULT_PROVIDER", "sprites")

	cfg, err := config.Load(config.LoadOptions{Provider: "modal"}, reg)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Provider != "modal" {
		t.Fatalf("provider = %q, want modal", cfg.Provider)
	}
}

func TestLoadFromEnv(t *testing.T) {
	reg := testRegistry(t)
	const token = "sprites-env-token" // #nosec G101 -- test fixture, not a real credential
	t.Setenv("KAXA_DEFAULT_PROVIDER", "sprites")
	t.Setenv("KAXA_SPRITES_TOKEN", token)
	t.Setenv("KAXA_MODAL_TOKEN_ID", "modal-id")
	t.Setenv("KAXA_MODAL_TOKEN_SECRET", "modal-secret")

	cfg, err := config.Load(config.LoadOptions{}, reg)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Provider != "sprites" {
		t.Fatalf("provider = %q, want sprites", cfg.Provider)
	}
	if cfg.Sprites.Token != token {
		t.Fatalf("sprites token = %q, want %q", cfg.Sprites.Token, token)
	}
	if cfg.Modal.TokenID != "modal-id" || cfg.Modal.TokenSecret != "modal-secret" {
		t.Fatalf("modal credentials not loaded from env")
	}
}

func TestValidateUnknownProvider(t *testing.T) {
	reg := testRegistry(t)
	const secret = "must-not-leak-token"
	t.Setenv("KAXA_SPRITES_TOKEN", secret)

	_, err := config.Load(config.LoadOptions{Provider: "unknown"}, reg)
	if err == nil {
		t.Fatal("expected unknown provider error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("error message must not contain token")
	}
}
