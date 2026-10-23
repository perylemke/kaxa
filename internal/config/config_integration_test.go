//go:build integration

package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/perylemke/sbx/internal/config"
)

func TestLoadXDGLayoutEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "sbx")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfgPath := filepath.Join(configDir, "config.yaml")
	const secret = "sprites-secret-token"
	content := "provider: modal\nsprites:\n  token: " + secret + "\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("SBX_PROVIDER", "sprites")

	reg := testRegistry(t)
	cfg, err := config.Load(config.LoadOptions{}, reg)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Provider != "sprites" {
		t.Fatalf("provider = %q, want sprites", cfg.Provider)
	}
	if cfg.Sprites.Token != secret {
		t.Fatalf("token not loaded from file")
	}

	_, err = config.Load(config.LoadOptions{Provider: "not-a-provider"}, reg)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("error message must not contain token")
	}
}
