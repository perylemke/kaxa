package lifecycle_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

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
	if got := lifecycle.Alternatives(reg, sandbox.FeatureRestart); !slices.Equal(got, []string{"alpha"}) {
		t.Fatalf("restart alternatives = %v, want [alpha]", got)
	}
	if got := lifecycle.Alternatives(reg, sandbox.FeatureUpgrade); len(got) != 0 {
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
			got := lifecycle.WithAlternatives(reg, tt.err)
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
	if got := lifecycle.WithAlternatives(altRegistry(t), want); !errors.Is(got, want) {
		t.Fatalf("err = %v, want unchanged", got)
	}
	if lifecycle.WithAlternatives(altRegistry(t), nil) != nil {
		t.Fatal("nil should stay nil")
	}
}
