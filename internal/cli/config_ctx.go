package cli

import (
	"context"
	"errors"

	"github.com/perylemke/kaxa/internal/config"
	"github.com/perylemke/kaxa/internal/registry"
)

type configKey struct{}

type registryKey struct{}

func withRegistry(ctx context.Context, reg *registry.Registry) context.Context {
	return context.WithValue(ctx, registryKey{}, reg)
}

// registryFromContext returns the registry NewRootCmd stored, if any. A nil
// context is accepted because cobra leaves it unset until a command runs.
func registryFromContext(ctx context.Context) (*registry.Registry, bool) {
	if ctx == nil {
		return nil, false
	}
	reg, ok := ctx.Value(registryKey{}).(*registry.Registry)
	return reg, ok && reg != nil
}

func withConfig(ctx context.Context, cfg config.Config) context.Context {
	return context.WithValue(ctx, configKey{}, cfg)
}

func configFromContext(ctx context.Context) (config.Config, error) {
	cfg, ok := ctx.Value(configKey{}).(config.Config)
	if !ok {
		return config.Config{}, errors.New("configuration not loaded")
	}
	return cfg, nil
}
