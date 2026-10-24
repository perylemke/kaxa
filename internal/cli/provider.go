package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/perylemke/sbox/internal/config"
	"github.com/perylemke/sbox/internal/registry"
	"github.com/perylemke/sbox/internal/sandbox"
	"github.com/perylemke/sbox/internal/usererr"
)

type tokenConfigurer interface {
	Configure(token string) error
}

func providerFromCmd(cmd *cobra.Command, reg *registry.Registry) (sandbox.Provider, error) {
	cfg, err := configFromContext(cmd.Context())
	if err != nil {
		return nil, err
	}
	return providerFromConfig(reg, cfg)
}

func providerFromConfig(reg *registry.Registry, cfg config.Config) (sandbox.Provider, error) {
	if cfg.Provider == "" {
		return nil, usererr.New(
			"missing sandbox provider",
			"pass --provider or set SBOX_DEFAULT_PROVIDER",
		)
	}

	p, err := reg.Get(cfg.Provider)
	if err != nil {
		return nil, err
	}

	if cfg.Provider == "sprites" {
		c, ok := p.(tokenConfigurer)
		if !ok {
			return nil, fmt.Errorf("sprites provider %T does not support Configure", p)
		}
		if err := c.Configure(cfg.Sprites.Token); err != nil {
			return nil, err
		}
	}

	return p, nil
}
