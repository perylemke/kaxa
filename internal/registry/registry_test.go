package registry_test

import (
	"testing"

	"github.com/perylemke/kaxa/internal/provider/modal"
	"github.com/perylemke/kaxa/internal/provider/sprites"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestRegisterGetNames(t *testing.T) {
	t.Parallel()

	reg := registry.New()
	if err := reg.Register(sprites.New()); err != nil {
		t.Fatalf("register sprites: %v", err)
	}
	if err := reg.Register(modal.New()); err != nil {
		t.Fatalf("register modal: %v", err)
	}

	if err := reg.Register(sprites.New()); err == nil {
		t.Fatal("expected duplicate register error")
	}

	got, err := reg.Get("sprites")
	if err != nil {
		t.Fatalf("get sprites: %v", err)
	}
	if got.Name() != "sprites" {
		t.Fatalf("name = %q, want sprites", got.Name())
	}

	names := reg.Names()
	if len(names) != 2 || names[0] != "modal" || names[1] != "sprites" {
		t.Fatalf("names = %v, want [modal sprites]", names)
	}
}

func TestGetUnknown(t *testing.T) {
	t.Parallel()

	reg := registry.New()
	if _, err := reg.Get("nope"); err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestAllIsSortedByName(t *testing.T) {
	t.Parallel()

	reg := registry.New()
	if got := reg.All(); len(got) != 0 {
		t.Fatalf("empty registry returned %d providers", len(got))
	}
	for _, p := range []sandbox.Provider{sprites.New(), modal.New()} {
		if err := reg.Register(p); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	if !reg.Has("modal") || reg.Has("missing") {
		t.Fatal("Has must report registered names only")
	}
	all := reg.All()
	if len(all) != 2 || all[0].Name() != "modal" || all[1].Name() != "sprites" {
		t.Fatalf("all = %v", all)
	}
}
