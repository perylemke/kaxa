// Package config loads kaxa settings from environment variables and CLI flags.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	envDefaultProvider  = "KAXA_DEFAULT_PROVIDER"
	envSpritesToken     = "KAXA_SPRITES_TOKEN"      // #nosec G101 -- env var name, not a credential value
	envModalTokenID     = "KAXA_MODAL_TOKEN_ID"     // #nosec G101 -- env var name, not a credential value
	envModalTokenSecret = "KAXA_MODAL_TOKEN_SECRET" // #nosec G101 -- env var name, not a credential value
	envE2BAPIKey        = "KAXA_E2B_API_KEY"        // #nosec G101 -- env var name, not a credential value
	envE2BTemplate      = "KAXA_E2B_TEMPLATE"
)

// Config holds resolved CLI settings (secrets are never logged via String).
type Config struct {
	Provider string
	Sprites  SpritesConfig
	Modal    ModalConfig
	E2B      E2BConfig
}

// E2BConfig holds the E2B API key and the template new sandboxes start from.
type E2BConfig struct {
	APIKey string
	// Template is the default template. Empty means E2B's base template.
	Template string
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
		E2B: E2BConfig{APIKey: os.Getenv(envE2BAPIKey), Template: os.Getenv(envE2BTemplate)},
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
		return usererr.New(
			fmt.Sprintf("unknown provider %q", cfg.Provider),
			"supported: "+strings.Join(reg.Names(), ", "),
		)
	}
	return nil
}
