// Package main is the kaxa CLI entrypoint.
package main

import (
	"fmt"
	"os"

	"github.com/perylemke/kaxa/internal/cli"
	"github.com/perylemke/kaxa/internal/provider/docker"
	"github.com/perylemke/kaxa/internal/provider/e2b"
	"github.com/perylemke/kaxa/internal/provider/modal"
	"github.com/perylemke/kaxa/internal/provider/sprites"
	"github.com/perylemke/kaxa/internal/registry"
)

func main() {
	reg := registry.New()
	for _, p := range []struct {
		name string
		reg  func() error
	}{
		{"sprites", func() error { return reg.Register(sprites.New()) }},
		{"modal", func() error { return reg.Register(modal.New()) }},
		{"e2b", func() error { return reg.Register(e2b.New()) }},
		{"docker", func() error { return reg.Register(docker.New()) }},
	} {
		if err := p.reg(); err != nil {
			_ = cli.WriteError(os.Stderr, fmt.Errorf("register %s: %w", p.name, err))
			os.Exit(1)
		}
	}

	os.Exit(cli.Execute(reg))
}
