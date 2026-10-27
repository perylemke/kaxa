package lifecycle_test

import (
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
