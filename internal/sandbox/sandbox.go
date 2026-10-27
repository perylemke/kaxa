// Package sandbox defines the provider contract shared by sandbox backends.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/perylemke/sbox/internal/project"
)

// ErrNotImplemented indicates a provider method is not yet implemented.
var ErrNotImplemented = errors.New("not implemented")

// SyncLabel marks a sandbox whose workspace was copied from a local Git repository.
const SyncLabel = "sync"

// ID identifies a sandbox instance.
type ID string

// Spec describes a sandbox to create.
type Spec struct {
	Name      string
	Labels    []string
	Running   bool
	ExposeAPI bool
}

// Info summarizes an existing sandbox.
type Info struct {
	ID       ID
	Provider string
	Status   string
	Labels   []string
}

// Detail is a full sandbox record for inspect and TUI detail views.
type Detail struct {
	ID           ID
	Provider     string
	Status       string
	Labels       []string
	SpriteUUID   string
	Organization string
	URL          string
	Auth         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
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

// WorkspaceSyncer copies a gzip tar into the sandbox workspace.
// The archive's top-level directory is the repository name.
type WorkspaceSyncer interface {
	SyncWorkspace(ctx context.Context, id ID, archive io.Reader) error
}

// ProjectSetup installs toolchains and project dependencies in a synced workspace.
type ProjectSetup interface {
	SetupProjects(ctx context.Context, id ID, repoName string, steps []project.Step) error
}

// APIPublisher starts an HTTP service and routes the sandbox URL to it.
type APIPublisher interface {
	PublishAPI(ctx context.Context, id ID, workDir string, port int, argv []string) error
}

// URLAuthPublic is a Sprites URL that anyone with the address can open.
const URLAuthPublic = "public"

// URLAuthSprite is a Sprites URL that requires a Sprites account.
const URLAuthSprite = "sprite"

// PublicURL changes whether a sandbox URL requires provider authentication.
type PublicURL interface {
	SetURLAuth(ctx context.Context, id ID, auth string) error
}

// Provider is a sandbox backend (Sprites, Modal, etc.).
type Provider interface {
	Name() string
	Create(ctx context.Context, spec Spec) (ID, error)
	List(ctx context.Context) ([]Info, error)
	Inspect(ctx context.Context, id ID) (Detail, error)
	Restart(ctx context.Context, id ID) error
	Upgrade(ctx context.Context, id ID) error
	Exec(ctx context.Context, id ID, opts ExecOptions) error
	Destroy(ctx context.Context, id ID) error
}
