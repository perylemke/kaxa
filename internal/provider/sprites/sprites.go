package sprites

import (
	"context"
	"fmt"

	"github.com/perylemke/sbx/internal/sandbox"
)

// Provider implements sandbox.Provider for Fly.io Sprites.
type Provider struct{}

// New returns a Sprites provider stub.
func New() *Provider {
	return &Provider{}
}

// Name returns the registry name for Sprites.
func (p *Provider) Name() string {
	return "sprites"
}

// Create is not implemented yet.
func (p *Provider) Create(ctx context.Context, spec sandbox.Spec) (sandbox.ID, error) {
	return "", fmt.Errorf("%w: sprites create", sandbox.ErrNotImplemented)
}

// List is not implemented yet.
func (p *Provider) List(ctx context.Context) ([]sandbox.Info, error) {
	return nil, fmt.Errorf("%w: sprites list", sandbox.ErrNotImplemented)
}

// Exec is not implemented yet.
func (p *Provider) Exec(ctx context.Context, id sandbox.ID, command []string) error {
	return fmt.Errorf("%w: sprites exec", sandbox.ErrNotImplemented)
}

// Destroy is not implemented yet.
func (p *Provider) Destroy(ctx context.Context, id sandbox.ID) error {
	return fmt.Errorf("%w: sprites destroy", sandbox.ErrNotImplemented)
}
