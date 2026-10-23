package main

import (
	"fmt"
	"os"

	"github.com/perylemke/sbx/internal/cli"
	"github.com/perylemke/sbx/internal/provider/modal"
	"github.com/perylemke/sbx/internal/provider/sprites"
	"github.com/perylemke/sbx/internal/registry"
)

func main() {
	reg := registry.New()
	for _, p := range []struct {
		name string
		reg  func() error
	}{
		{"sprites", func() error { return reg.Register(sprites.New()) }},
		{"modal", func() error { return reg.Register(modal.New()) }},
	} {
		if err := p.reg(); err != nil {
			fmt.Fprintf(os.Stderr, "register %s: %v\n", p.name, err)
			os.Exit(1)
		}
	}

	os.Exit(cli.Execute(reg))
}
