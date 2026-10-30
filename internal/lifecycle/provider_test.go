package lifecycle_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/config"
	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

type namedProvider struct {
	basicProvider
	name        string
	token       string
	template    string
	templateErr error
}

func (n *namedProvider) Name() string { return n.name }

func (n *namedProvider) SetDefaultTemplate(name string) error {
	n.template = name
	return n.templateErr
}

func (n *namedProvider) Configure(token string) error {
	n.token = token
	return nil
}

func registryWith(t *testing.T, ps ...sandbox.Provider) *registry.Registry {
	t.Helper()
	reg := registry.New()
	for _, p := range ps {
		if err := reg.Register(p); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	return reg
}

func TestResolveProviderConfiguresSprites(t *testing.T) {
	t.Parallel()

	p := &namedProvider{name: listSprites}
	reg := registryWith(t, p)
	cfg := config.Config{Sprites: config.SpritesConfig{Token: "tok"}}

	got, err := lifecycle.ResolveProvider(reg, cfg, listSprites)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != sandbox.Provider(p) || p.token != "tok" {
		t.Fatalf("provider = %v token = %q", got, p.token)
	}
}

func TestResolveProviderConfiguresE2B(t *testing.T) {
	t.Parallel()

	p := &namedProvider{name: e2bName}
	reg := registryWith(t, p)
	cfg := config.Config{E2B: config.E2BConfig{APIKey: "key", Template: bigTpl}}

	got, err := lifecycle.ResolveProvider(reg, cfg, e2bName)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != sandbox.Provider(p) || p.token != "key" || p.template != "big" {
		t.Fatalf("provider = %v key = %q template = %q", got, p.token, p.template)
	}
}

func TestResolveProviderRejectsBadE2BTemplate(t *testing.T) {
	t.Parallel()

	want := errors.New("bad template")
	reg := registryWith(t, &namedProvider{name: e2bName, templateErr: want})
	cfg := config.Config{E2B: config.E2BConfig{APIKey: "key", Template: "x y"}}

	if _, err := lifecycle.ResolveProvider(reg, cfg, e2bName); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestResolveProviderLeavesOthersAlone(t *testing.T) {
	t.Parallel()

	p := &namedProvider{name: "other"}
	reg := registryWith(t, p)

	if _, err := lifecycle.ResolveProvider(reg, config.Config{}, "other"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if p.token != "" {
		t.Fatalf("token = %q", p.token)
	}
}

func TestResolveProviderErrors(t *testing.T) {
	t.Parallel()

	reg := registryWith(t, &namedProvider{name: listSprites})

	_, err := lifecycle.ResolveProvider(reg, config.Config{}, "")
	if ue, ok := usererr.As(err); !ok || !strings.Contains(ue.Message, "missing sandbox provider") {
		t.Fatalf("empty name err = %v", err)
	}
	if _, err := lifecycle.ResolveProvider(reg, config.Config{}, "nope"); err == nil {
		t.Fatal("unknown provider accepted")
	}
}

type credProvider struct {
	basicProvider
	id, secret string
	err        error
}

func (c *credProvider) Name() string { return listModal }

func (c *credProvider) ConfigureCredentials(id, secret string) error {
	c.id, c.secret = id, secret
	return c.err
}

func TestResolveProviderConfiguresModal(t *testing.T) {
	t.Parallel()

	p := &credProvider{}
	reg := registryWith(t, p)
	cfg := config.Config{Modal: config.ModalConfig{TokenID: "ak-1", TokenSecret: "as-2"}}

	got, err := lifecycle.ResolveProvider(reg, cfg, listModal)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != sandbox.Provider(p) || p.id != "ak-1" || p.secret != "as-2" {
		t.Fatalf("provider = %v id = %q secret = %q", got, p.id, p.secret)
	}
}

func TestResolveProviderModalConfigureError(t *testing.T) {
	t.Parallel()

	want := errors.New("missing credentials")
	reg := registryWith(t, &credProvider{err: want})

	if _, err := lifecycle.ResolveProvider(reg, config.Config{}, listModal); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestResolveProviderModalWithoutConfigurer(t *testing.T) {
	t.Parallel()

	reg := registryWith(t, &namedProvider{name: listModal})

	if _, err := lifecycle.ResolveProvider(reg, config.Config{}, listModal); err == nil {
		t.Fatal("modal provider without ConfigureCredentials accepted")
	}
}

type dockerProvider struct {
	basicProvider
	id, pat     string
	template    string
	configErr   error
	templateErr error
}

func (c *dockerProvider) Name() string { return "docker" }

func (c *dockerProvider) ConfigureCredentials(id, pat string) error {
	c.id, c.pat = id, pat
	return c.configErr
}

func (c *dockerProvider) SetDefaultTemplate(name string) error {
	c.template = name
	return c.templateErr
}

func TestResolveProviderConfiguresDocker(t *testing.T) {
	t.Parallel()

	p := &dockerProvider{}
	cfg := config.Config{Docker: config.DockerConfig{ID: "me", PAT: "dckr_pat_x", Template: "ubuntu:24.04"}}

	if _, err := lifecycle.ResolveProvider(registryWith(t, p), cfg, "docker"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if p.id != "me" || p.pat != "dckr_pat_x" || p.template != "ubuntu:24.04" {
		t.Fatalf("id = %q pat = %q template = %q", p.id, p.pat, p.template)
	}
}

func TestResolveProviderDockerErrors(t *testing.T) {
	t.Parallel()

	want := errors.New("failure")
	tests := []struct {
		name string
		p    sandbox.Provider
	}{
		{"credentials", &dockerProvider{configErr: want}},
		{"template", &dockerProvider{templateErr: want}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := lifecycle.ResolveProvider(registryWith(t, tt.p), config.Config{}, "docker"); !errors.Is(err, want) {
				t.Fatalf("err = %v, want %v", err, want)
			}
		})
	}

	reg := registryWith(t, &namedProvider{name: "docker"})
	if _, err := lifecycle.ResolveProvider(reg, config.Config{}, "docker"); err == nil {
		t.Fatal("docker provider without ConfigureCredentials accepted")
	}
}
