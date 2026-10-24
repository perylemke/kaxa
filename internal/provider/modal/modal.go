// Package modal is the Modal sandbox provider.
package modal

import (
	"context"

	"github.com/perylemke/sbox/internal/sandbox"
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
func (p *Provider) Create(_ context.Context, _ sandbox.Spec) (sandbox.ID, error) {
	return "", errNotImplemented("create")
}

// List is not implemented yet.
func (p *Provider) List(_ context.Context) ([]sandbox.Info, error) {
	return nil, errNotImplemented("list")
}

// Exec is not implemented yet.
func (p *Provider) Exec(_ context.Context, _ sandbox.ID, _ sandbox.ExecOptions) error {
	return errNotImplemented("exec")
}

// Destroy is not implemented yet.
func (p *Provider) Destroy(_ context.Context, _ sandbox.ID) error {
	return errNotImplemented("destroy")
}
