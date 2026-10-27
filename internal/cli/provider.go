package cli

import (
	"github.com/spf13/cobra"

	"github.com/perylemke/sbox/internal/config"
	"github.com/perylemke/sbox/internal/lifecycle"
	"github.com/perylemke/sbox/internal/registry"
	"github.com/perylemke/sbox/internal/sandbox"
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
