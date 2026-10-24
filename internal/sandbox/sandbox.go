// Package sandbox defines the provider contract shared by sandbox backends.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	Status   string
}

// TermSize is terminal dimensions in rows and columns.
type TermSize struct {
	Rows uint16
	Cols uint16
}

// ExecOptions configures remote command execution.
type ExecOptions struct {
	Command []string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	TTY     bool
	Size    TermSize
	Resize  <-chan TermSize
}

// ExitError reports a non-zero exit from a remote command.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit status %d", e.Code)
}

// Provider is a sandbox backend (Sprites, Modal, etc.).
type Provider interface {
	Name() string
	Create(ctx context.Context, spec Spec) (ID, error)
	List(ctx context.Context) ([]Info, error)
	Exec(ctx context.Context, id ID, opts ExecOptions) error
	Destroy(ctx context.Context, id ID) error
}
