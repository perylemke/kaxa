package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/perylemke/sbx/internal/config"
	"github.com/perylemke/sbx/internal/provider/modal"
	"github.com/perylemke/sbx/internal/provider/sprites"
	"github.com/perylemke/sbx/internal/registry"
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
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("provider: modal\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("SBX_PROVIDER", "sprites")

	cfg, err := config.Load(config.LoadOptions{
		ConfigPath: cfgPath,
		Provider:   "modal",
	}, reg)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Provider != "modal" {
		t.Fatalf("provider = %q, want modal", cfg.Provider)
	}
}

func TestValidateUnknownProvider(t *testing.T) {
	t.Parallel()

	reg := testRegistry(t)
	_, err := config.Load(config.LoadOptions{Provider: "unknown"}, reg)
	if err == nil {
		t.Fatal("expected unknown provider error")
	}
}

func TestLoadMissingExplicitConfig(t *testing.T) {
	t.Parallel()

	reg := testRegistry(t)
	_, err := config.Load(config.LoadOptions{ConfigPath: "/no/such/config.yaml"}, reg)
	if err == nil {
		t.Fatal("expected missing config error")
	}
}
