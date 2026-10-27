package cli

import (
	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/config"
	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
)

const spritesProviderName = "sprites"

func providerFromCmd(cmd *cobra.Command, reg *registry.Registry) (sandbox.Provider, error) {
	cfg, err := configFromContext(cmd.Context())
	if err != nil {
		return nil, err
	}
	return providerFromConfig(reg, cfg)
}

func providerFromConfig(reg *registry.Registry, cfg config.Config) (sandbox.Provider, error) {
	return lifecycle.ResolveProvider(reg, cfg, cfg.Provider)
}
