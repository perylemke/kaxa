package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"

	"github.com/perylemke/sbx/internal/registry"
)

// Config holds resolved CLI settings (secrets are never logged via String).
type Config struct {
	Provider string       `mapstructure:"provider"`
	Sprites  SpritesConfig `mapstructure:"sprites"`
	Modal    ModalConfig   `mapstructure:"modal"`
}

// SpritesConfig holds Fly.io Sprites credentials.
type SpritesConfig struct {
	Token string `mapstructure:"token"`
}

// ModalConfig holds Modal API credentials.
type ModalConfig struct {
	TokenID     string `mapstructure:"token_id"`
	TokenSecret string `mapstructure:"token_secret"`
}

// LoadOptions are flag overrides passed from the CLI.
type LoadOptions struct {
	ConfigPath string
	Provider   string
}

// Load reads configuration from file, environment, and flags.
func Load(opts LoadOptions, reg *registry.Registry) (Config, error) {
	v := viper.New()
	v.SetEnvPrefix("SBX")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetDefault("provider", "")
	v.SetDefault("sprites.token", "")
	v.SetDefault("modal.token_id", "")
	v.SetDefault("modal.token_secret", "")

	if err := readConfigFile(v, opts.ConfigPath); err != nil {
		return Config{}, err
	}

	if opts.Provider != "" {
		v.Set("provider", opts.Provider)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
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

func readConfigFile(v *viper.Viper, explicitPath string) error {
	if explicitPath != "" {
		if _, err := os.Stat(explicitPath); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("config file not found: %s", explicitPath)
			}
			return fmt.Errorf("config file: %w", err)
		}
		v.SetConfigFile(explicitPath)
		if err := v.ReadInConfig(); err != nil {
			return fmt.Errorf("read config: %w", err)
		}
		return nil
	}

	for _, dir := range searchDirs() {
		path := filepath.Join(dir, "config.yaml")
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("config file: %w", err)
		}
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			var notFound viper.ConfigFileNotFoundError
			if errors.As(err, &notFound) {
				continue
			}
			return fmt.Errorf("read config: %w", err)
		}
		return nil
	}
	return nil
}

func searchDirs() []string {
	var dirs []string
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		dirs = append(dirs, filepath.Join(xdg, "sbx"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".config", "sbx"))
	}
	return dirs
}
