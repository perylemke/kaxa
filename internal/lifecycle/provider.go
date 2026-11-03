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
	dockerProvider  = "docker"
	daytonaProvider = "daytona"
	blaxelProvider  = "blaxel"
)

type tokenConfigurer interface {
	Configure(token string) error
}

// templateDefaulter is a provider that starts sandboxes from a default template.
type templateDefaulter interface {
	SetDefaultTemplate(name string) error
}

// blaxelAccount is a provider configured with a Blaxel API key and workspace.
type blaxelAccount interface {
	Configure(apiKey, workspace string) error
	SetRegion(region string) error
	SetMemory(mb int) error
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

	if err := configureProvider(p, name, cfg); err != nil {
		return nil, err
	}

	return p, nil
}

// configureProvider applies the credentials and defaults of cfg that belong to
// the provider called name. A provider with none is left alone.
func configureProvider(p sandbox.Provider, name string, cfg config.Config) error {
	switch name {
	case spritesProvider:
		return configureSprites(p, cfg.Sprites)
	case modalProvider:
		return configureModal(p, cfg.Modal)
	case e2bProvider:
		if err := configureE2B(p, cfg.E2B); err != nil {
			return err
		}
	case dockerProvider:
		if err := configureDocker(p, cfg.Docker); err != nil {
			return err
		}
	case daytonaProvider:
		if err := configureDaytona(p, cfg.Daytona); err != nil {
			return err
		}
	case blaxelProvider:
		if err := configureBlaxel(p, cfg.Blaxel); err != nil {
			return err
		}
	}

	return nil
}

func configureSprites(p sandbox.Provider, cfg config.SpritesConfig) error {
	c, ok := p.(tokenConfigurer)
	if !ok {
		return fmt.Errorf("sprites provider %T does not support Configure", p)
	}
	return c.Configure(cfg.Token)
}

func configureModal(p sandbox.Provider, cfg config.ModalConfig) error {
	c, ok := p.(credentialsConfigurer)
	if !ok {
		return fmt.Errorf("modal provider %T does not support ConfigureCredentials", p)
	}
	return c.ConfigureCredentials(cfg.TokenID, cfg.TokenSecret)
}

func configureDaytona(p sandbox.Provider, cfg config.DaytonaConfig) error {
	c, ok := p.(tokenConfigurer)
	if !ok {
		return fmt.Errorf("daytona provider %T does not support Configure", p)
	}
	if err := c.Configure(cfg.APIKey); err != nil {
		return err
	}
	if d, ok := p.(templateDefaulter); ok {
		return d.SetDefaultTemplate(cfg.Template)
	}
	return nil
}

func configureBlaxel(p sandbox.Provider, cfg config.BlaxelConfig) error {
	c, ok := p.(blaxelAccount)
	if !ok {
		return fmt.Errorf("blaxel provider %T does not support Configure", p)
	}
	if err := c.Configure(cfg.APIKey, cfg.Workspace); err != nil {
		return err
	}
	if err := c.SetRegion(cfg.Region); err != nil {
		return err
	}
	if err := c.SetMemory(cfg.MemoryMB); err != nil {
		return err
	}
	if d, ok := p.(templateDefaulter); ok {
		return d.SetDefaultTemplate(cfg.Template)
	}
	return nil
}

func configureDocker(p sandbox.Provider, cfg config.DockerConfig) error {
	c, ok := p.(credentialsConfigurer)
	if !ok {
		return fmt.Errorf("docker provider %T does not support ConfigureCredentials", p)
	}
	if err := c.ConfigureCredentials(cfg.ID, cfg.PAT); err != nil {
		return err
	}
	if d, ok := p.(templateDefaulter); ok {
		return d.SetDefaultTemplate(cfg.Template)
	}
	return nil
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
