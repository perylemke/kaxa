// Package config loads sbx settings from environment variables and CLI flags.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/perylemke/sbx/internal/registry"
)

const (
	envDefaultProvider = "SBX_DEFAULT_PROVIDER"
	envSpritesToken    = "SBX_SPRITES_TOKEN"
	envModalTokenID    = "SBX_MODAL_TOKEN_ID"
	envModalTokenSecret  = "SBX_MODAL_TOKEN_SECRET"
)

// Config holds resolved CLI settings (secrets are never logged via String).
type Config struct {
	Provider string
	Sprites  SpritesConfig
	Modal    ModalConfig
}

// SpritesConfig holds Fly.io Sprites credentials.
type SpritesConfig struct {
	Token string
}

// ModalConfig holds Modal API credentials.
type ModalConfig struct {
	TokenID     string
	TokenSecret string
}

// LoadOptions are flag overrides passed from the CLI.
type LoadOptions struct {
	Provider string
}

// Load reads configuration from environment variables and applies flag overrides.
func Load(opts LoadOptions, reg *registry.Registry) (Config, error) {
	cfg := Config{
		Provider: os.Getenv(envDefaultProvider),
		Sprites: SpritesConfig{
			Token: os.Getenv(envSpritesToken),
		},
		Modal: ModalConfig{
			TokenID:     os.Getenv(envModalTokenID),
			TokenSecret: os.Getenv(envModalTokenSecret),
		},
	}

	if opts.Provider != "" {
		cfg.Provider = opts.Provider
	}

	if err := Validate(&cfg, reg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// Validate checks provider names against the registry.
func Validate(cfg *Config, reg *registry.Registry) error {
	if cfg.Provider == "" {
		return nil
	}
	if !reg.Has(cfg.Provider) {
		return fmt.Errorf("unknown provider %q (supported: %s)", cfg.Provider, strings.Join(reg.Names(), ", "))
	}
	return nil
}
