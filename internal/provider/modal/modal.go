package modal

import (
	"context"
	"fmt"

	"github.com/perylemke/sbx/internal/sandbox"
)

// Provider implements sandbox.Provider for Modal sandboxes.
type Provider struct{}

// New returns a Modal provider stub.
func New() *Provider {
	return &Provider{}
}

// Name returns the registry name for Modal.
func (p *Provider) Name() string {
	return "modal"
}

// Create is not implemented yet.
func (p *Provider) Create(ctx context.Context, spec sandbox.Spec) (sandbox.ID, error) {
	return "", fmt.Errorf("%w: modal create", sandbox.ErrNotImplemented)
}

// List is not implemented yet.
func (p *Provider) List(ctx context.Context) ([]sandbox.Info, error) {
	return nil, fmt.Errorf("%w: modal list", sandbox.ErrNotImplemented)
}

// Exec is not implemented yet.
func (p *Provider) Exec(ctx context.Context, id sandbox.ID, command []string) error {
	return fmt.Errorf("%w: modal exec", sandbox.ErrNotImplemented)
}

// Destroy is not implemented yet.
func (p *Provider) Destroy(ctx context.Context, id sandbox.ID) error {
	return fmt.Errorf("%w: modal destroy", sandbox.ErrNotImplemented)
}
