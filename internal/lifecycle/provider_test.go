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
	name  string
	token string
}

func (n *namedProvider) Name() string { return n.name }

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

	p := &namedProvider{name: "sprites"}
	reg := registryWith(t, p)
	cfg := config.Config{Sprites: config.SpritesConfig{Token: "tok"}}

	got, err := lifecycle.ResolveProvider(reg, cfg, "sprites")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != sandbox.Provider(p) || p.token != "tok" {
		t.Fatalf("provider = %v token = %q", got, p.token)
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

	reg := registryWith(t, &namedProvider{name: "sprites"})

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

func (c *credProvider) Name() string { return "modal" }

func (c *credProvider) ConfigureCredentials(id, secret string) error {
	c.id, c.secret = id, secret
	return c.err
}

func TestResolveProviderConfiguresModal(t *testing.T) {
	t.Parallel()

	p := &credProvider{}
	reg := registryWith(t, p)
	cfg := config.Config{Modal: config.ModalConfig{TokenID: "ak-1", TokenSecret: "as-2"}}

	got, err := lifecycle.ResolveProvider(reg, cfg, "modal")
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

	if _, err := lifecycle.ResolveProvider(reg, config.Config{}, "modal"); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestResolveProviderModalWithoutConfigurer(t *testing.T) {
	t.Parallel()

	reg := registryWith(t, &namedProvider{name: "modal"})

	if _, err := lifecycle.ResolveProvider(reg, config.Config{}, "modal"); err == nil {
		t.Fatal("modal provider without ConfigureCredentials accepted")
	}
}
