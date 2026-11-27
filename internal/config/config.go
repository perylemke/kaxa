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
	envBlaxelDisk       = "KAXA_BLAXEL_DISK_PERCENT"
	envVercelToken      = "KAXA_VERCEL_TOKEN" // #nosec G101 -- env var name, not a credential value
	envVercelProject    = "KAXA_VERCEL_PROJECT"
	envVercelTeam       = "KAXA_VERCEL_TEAM"
	envVercelTemplate   = "KAXA_VERCEL_TEMPLATE"
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
	// Vercel configures the vercel provider.
	Vercel VercelConfig
}

// VercelConfig holds the Vercel access token, the project that owns the
// sandboxes, the team when the token belongs to one, and the default image.
type VercelConfig struct {
	Token string
	// Project is the name or ID of the Vercel project.
	Project string
	// Team is the team ID (team_...) or slug. Empty means the personal account.
	Team string
	// Template is the image new sandboxes start from. Empty means the Vercel default.
	Template string
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
	// DiskPercent is the share of that memory the filesystem may use, from 10 to 95.
	// Zero omits the field and Blaxel uses 50.
	DiskPercent int
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
		Vercel: VercelConfig{
			Token:    os.Getenv(envVercelToken),
			Project:  os.Getenv(envVercelProject),
			Team:     os.Getenv(envVercelTeam),
			Template: os.Getenv(envVercelTemplate),
		},
	}
	memory, err := blaxelMemory(os.Getenv(envBlaxelMemory))
	if err != nil {
		return Config{}, err
	}
	cfg.Blaxel.MemoryMB = memory
	disk, err := blaxelDiskPercent(os.Getenv(envBlaxelDisk))
	if err != nil {
		return Config{}, err
	}
	cfg.Blaxel.DiskPercent = disk

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

// blaxelDiskPercent parses KAXA_BLAXEL_DISK_PERCENT. An empty value omits the field.
func blaxelDiskPercent(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 10 || n > 95 {
		return 0, usererr.New(
			fmt.Sprintf("invalid Blaxel disk percent %q", raw),
			"set KAXA_BLAXEL_DISK_PERCENT to a whole number from 10 to 95, or leave it unset",
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
