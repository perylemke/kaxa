package cli

import (
	"context"
	"errors"

	"github.com/perylemke/kaxa/internal/config"
)

type configKey struct{}

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
