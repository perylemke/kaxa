// Package main is the sbox CLI entrypoint.
package main

import (
	"fmt"
	"os"

	"github.com/perylemke/sbox/internal/cli"
	"github.com/perylemke/sbox/internal/provider/modal"
	"github.com/perylemke/sbox/internal/provider/sprites"
	"github.com/perylemke/sbox/internal/registry"
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
			_ = cli.WriteError(os.Stderr, fmt.Errorf("register %s: %w", p.name, err))
			os.Exit(1)
		}
	}

	os.Exit(cli.Execute(reg))
}
