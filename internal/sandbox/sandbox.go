package sandbox

import (
	"context"
	"errors"
)

// ErrNotImplemented indicates a provider method is not yet implemented.
var ErrNotImplemented = errors.New("not implemented")

// ID identifies a sandbox instance.
type ID string

// Spec describes a sandbox to create.
type Spec struct {
	Name string
}

// Info summarizes an existing sandbox.
type Info struct {
	ID       ID
	Provider string
}

// Provider is a sandbox backend (Sprites, Modal, etc.).
type Provider interface {
	Name() string
	Create(ctx context.Context, spec Spec) (ID, error)
	List(ctx context.Context) ([]Info, error)
	Exec(ctx context.Context, id ID, command []string) error
	Destroy(ctx context.Context, id ID) error
}
