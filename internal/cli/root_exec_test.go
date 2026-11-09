package cli

import (
	"os"
	"testing"

	"github.com/perylemke/kaxa/internal/provider/modal"
	"github.com/perylemke/kaxa/internal/provider/sprites"
	"github.com/perylemke/kaxa/internal/registry"
)

func TestExecuteVersionReturnsZero(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(sprites.New()); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(modal.New()); err != nil {
		t.Fatal(err)
	}

	old := os.Args
	t.Cleanup(func() { os.Args = old })
	os.Args = []string{"kaxa", "version"}

	code := Execute(reg)
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
}
