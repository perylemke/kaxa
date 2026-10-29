package lifecycle

import (
	"fmt"

	"github.com/perylemke/kaxa/internal/config"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	spritesProvider = "sprites"
	modalProvider   = "modal"
	e2bProvider     = "e2b"
)

type tokenConfigurer interface {
	Configure(token string) error
}

// templateDefaulter is a provider that starts sandboxes from a default template.
type templateDefaulter interface {
	SetDefaultTemplate(name string) error
}

type credentialsConfigurer interface {
	ConfigureCredentials(tokenID, tokenSecret string) error
}

// ResolveProvider returns the registered provider called name, with the
// credentials from cfg applied. An empty name is a user error.
func ResolveProvider(reg *registry.Registry, cfg config.Config, name string) (sandbox.Provider, error) {
	if name == "" {
		return nil, usererr.New(
			"missing sandbox provider",
			"pass --provider or set KAXA_DEFAULT_PROVIDER",
		)
	}

	p, err := reg.Get(name)
	if err != nil {
		return nil, err
	}

	switch name {
	case spritesProvider:
		c, ok := p.(tokenConfigurer)
		if !ok {
			return nil, fmt.Errorf("sprites provider %T does not support Configure", p)
		}
		if err := c.Configure(cfg.Sprites.Token); err != nil {
			return nil, err
		}
	case modalProvider:
		c, ok := p.(credentialsConfigurer)
		if !ok {
			return nil, fmt.Errorf("modal provider %T does not support ConfigureCredentials", p)
		}
		if err := c.ConfigureCredentials(cfg.Modal.TokenID, cfg.Modal.TokenSecret); err != nil {
			return nil, err
		}
	case e2bProvider:
		if err := configureE2B(p, cfg.E2B); err != nil {
			return nil, err
		}
	}

	return p, nil
}

func configureE2B(p sandbox.Provider, cfg config.E2BConfig) error {
	c, ok := p.(tokenConfigurer)
	if !ok {
		return fmt.Errorf("e2b provider %T does not support Configure", p)
	}
	if err := c.Configure(cfg.APIKey); err != nil {
		return err
	}
	if d, ok := p.(templateDefaulter); ok {
		return d.SetDefaultTemplate(cfg.Template)
	}
	return nil
}
