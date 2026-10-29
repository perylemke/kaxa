package lifecycle_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/perylemke/kaxa/internal/config"
	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

type altProvider struct {
	name        string
	unavailable bool
}

func (a *altProvider) Name() string    { return a.name }
func (a *altProvider) Available() bool { return !a.unavailable }
func (*altProvider) Create(context.Context, sandbox.Spec) (sandbox.ID, error) {
	return "", nil
}
func (*altProvider) List(context.Context) ([]sandbox.Info, error) { return nil, nil }
func (*altProvider) Inspect(context.Context, sandbox.ID) (sandbox.Detail, error) {
	return sandbox.Detail{}, nil
}
func (*altProvider) Exec(context.Context, sandbox.ID, sandbox.ExecOptions) error { return nil }
func (*altProvider) Destroy(context.Context, sandbox.ID) error                   { return nil }

type restartingProvider struct{ altProvider }

func (*restartingProvider) Restart(context.Context, sandbox.ID) error { return nil }

func altRegistry(t *testing.T) *registry.Registry {
	t.Helper()
	reg := registry.New()
	for _, p := range []sandbox.Provider{
		&restartingProvider{altProvider{name: "alpha"}},
		&restartingProvider{altProvider{name: "stub", unavailable: true}},
		&altProvider{name: testPlain},
	} {
		if err := reg.Register(p); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	return reg
}

func TestAlternatives(t *testing.T) {
	t.Parallel()

	reg := altRegistry(t)
	if got := lifecycle.Alternatives(reg, config.Config{}, sandbox.FeatureRestart); !slices.Equal(got, []string{"alpha"}) {
		t.Fatalf("restart alternatives = %v, want [alpha]", got)
	}
	if got := lifecycle.Alternatives(reg, config.Config{}, sandbox.FeatureUpgrade); len(got) != 0 {
		t.Fatalf("upgrade alternatives = %v, want none", got)
	}
}

func TestWithAlternatives(t *testing.T) {
	t.Parallel()

	reg := altRegistry(t)
	plain := &altProvider{name: testPlain}

	tests := []struct {
		name     string
		err      error
		wantHint string
	}{
		{"names providers", lifecycle.Unsupported(plain, sandbox.FeatureRestart), "available for: alpha"},
		{"none left", lifecycle.Unsupported(plain, sandbox.FeatureUpgrade), "no other provider supports it yet"},
		{"wrapped", fmt.Errorf("run: %w", lifecycle.Unsupported(plain, sandbox.FeatureRestart)), "available for: alpha"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := lifecycle.WithAlternatives(reg, config.Config{}, tt.err)
			if !errors.Is(got, sandbox.ErrUnsupported) {
				t.Fatalf("err = %v, want ErrUnsupported", got)
			}
			ue, ok := usererr.As(got)
			if !ok || ue.Hint != tt.wantHint {
				t.Fatalf("hint = %+v, want %q", ue, tt.wantHint)
			}
		})
	}
}

func TestWithAlternativesPassesOtherErrors(t *testing.T) {
	t.Parallel()

	want := errors.New("boom")
	if got := lifecycle.WithAlternatives(altRegistry(t), config.Config{}, want); !errors.Is(got, want) {
		t.Fatalf("err = %v, want unchanged", got)
	}
	if lifecycle.WithAlternatives(altRegistry(t), config.Config{}, nil) != nil {
		t.Fatal("nil should stay nil")
	}
}

// keyedProvider reports itself available only after Configure, like E2B.
type keyedProvider struct {
	restartingProvider
	key string
}

func (k *keyedProvider) Available() bool { return k.key != "" }

func (k *keyedProvider) Configure(key string) error {
	k.key = key
	return nil
}

func TestAlternativesAppliesCredentials(t *testing.T) {
	t.Parallel()

	newReg := func() *registry.Registry {
		reg := registry.New()
		if err := reg.Register(&keyedProvider{restartingProvider: restartingProvider{altProvider{name: "e2b"}}}); err != nil {
			t.Fatalf("register: %v", err)
		}
		return reg
	}

	got := lifecycle.Alternatives(newReg(), config.Config{E2B: config.E2BConfig{APIKey: "k"}}, sandbox.FeatureRestart)
	if !slices.Equal(got, []string{"e2b"}) {
		t.Fatalf("with a key: alternatives = %v, want [e2b]", got)
	}
	if got := lifecycle.Alternatives(newReg(), config.Config{}, sandbox.FeatureRestart); len(got) != 0 {
		t.Fatalf("without a key: alternatives = %v, want none", got)
	}
}
