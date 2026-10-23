package registry_test

import (
	"testing"

	"github.com/perylemke/sbx/internal/provider/modal"
	"github.com/perylemke/sbx/internal/provider/sprites"
	"github.com/perylemke/sbx/internal/registry"
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
