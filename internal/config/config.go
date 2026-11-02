// Package config loads kaxa settings from environment variables and CLI flags.
package config

import (
	"fmt"
	"os"
	"strconv"
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
	envDockerID         = "KAXA_DOCKER_ID"
	envDockerPAT        = "KAXA_DOCKER_PAT" // #nosec G101 -- env var name, not a credential value
	envDockerTpl        = "KAXA_DOCKER_TEMPLATE"
	envDaytonaAPIKey    = "KAXA_DAYTONA_API_KEY" // #nosec G101 -- env var name, not a credential value
	envDaytonaTemplate  = "KAXA_DAYTONA_TEMPLATE"
	envBlaxelAPIKey     = "KAXA_BLAXEL_API_KEY" // #nosec G101 -- env var name, not a credential value
	envBlaxelWorkspace  = "KAXA_BLAXEL_WORKSPACE"
	envBlaxelTemplate   = "KAXA_BLAXEL_TEMPLATE"
	envBlaxelRegion     = "KAXA_BLAXEL_REGION"
	envBlaxelMemory     = "KAXA_BLAXEL_MEMORY"
)

// Config holds resolved CLI settings (secrets are never logged via String).
type Config struct {
	Provider string
	Sprites  SpritesConfig
	Modal    ModalConfig
	E2B      E2BConfig
	// Docker configures the docker provider.
	Docker DockerConfig
	// Daytona configures the daytona provider.
	Daytona DaytonaConfig
	// Blaxel configures the blaxel provider.
	Blaxel BlaxelConfig
}

// BlaxelConfig holds the Blaxel API key, workspace, and the defaults for a new sandbox.
type BlaxelConfig struct {
	APIKey    string
	Workspace string
	// Template is the image new sandboxes start from. Empty means blaxel/base-image:latest.
	Template string
	// Region is the deployment region. Empty means us-pdx-1.
	Region string
	// MemoryMB is the memory of a new sandbox, in megabytes. Zero means 4096.
	MemoryMB int
}

// DaytonaConfig holds the Daytona API key and the image new daytona sandboxes
// start from.
type DaytonaConfig struct {
	APIKey   string
	Template string
}

// DockerConfig holds the Docker credentials and the image new docker
// sandboxes start from.
type DockerConfig struct {
	// ID is the Docker ID, the user name of the account.
	ID string
	// PAT is a personal access token created with Allow Cloud Sandboxes access.
	PAT string
	// Template is the default image. Empty means the image of the shell agent.
	Template string
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
		Docker: DockerConfig{
			ID:       os.Getenv(envDockerID),
			PAT:      os.Getenv(envDockerPAT),
			Template: os.Getenv(envDockerTpl),
		},
		Daytona: DaytonaConfig{APIKey: os.Getenv(envDaytonaAPIKey), Template: os.Getenv(envDaytonaTemplate)},
		Blaxel:  BlaxelConfig{APIKey: os.Getenv(envBlaxelAPIKey), Workspace: os.Getenv(envBlaxelWorkspace), Template: os.Getenv(envBlaxelTemplate), Region: os.Getenv(envBlaxelRegion)},
	}
	memory, err := blaxelMemory(os.Getenv(envBlaxelMemory))
	if err != nil {
		return Config{}, err
	}
	cfg.Blaxel.MemoryMB = memory

	if opts.Provider != "" {
		cfg.Provider = opts.Provider
	}

	if err := Validate(&cfg, reg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// blaxelMemory parses KAXA_BLAXEL_MEMORY. An empty value means the provider default.
func blaxelMemory(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, usererr.New(
			fmt.Sprintf("invalid Blaxel memory %q", raw),
			"set KAXA_BLAXEL_MEMORY to a whole number of megabytes, such as 4096",
		)
	}
	return n, nil
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
