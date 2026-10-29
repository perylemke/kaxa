// Package sandbox defines the provider contract shared by sandbox backends.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/usererr"
)

// ErrNotImplemented indicates a provider method is not yet implemented.
var ErrNotImplemented = errors.New("not implemented")

// ErrNotConfigured indicates a provider has no credentials. Commands that act
// on every provider skip a provider that returns it.
var ErrNotConfigured = errors.New("provider not configured")

// SyncLabel marks a sandbox whose workspace was copied from a local Git repository.
const SyncLabel = "sync"

// TerminalLabel marks a sandbox whose URL serves the tmux terminal, which has no login.
const TerminalLabel = "terminal"

// PublicTerminalError reports that a public URL would hand the unauthenticated
// terminal of the sandbox to anyone who has the address.
func PublicTerminalError() error {
	return usererr.New(
		"a public URL would expose the unauthenticated tmux terminal",
		"keep the URL behind the provider account, or publish an API so the service owns the URL",
	)
}

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

// RepoChecker reports whether the directory SyncWorkspace would fill for repo
// already exists in a sandbox. repo must be a single path segment.
type RepoChecker interface {
	RepoExists(ctx context.Context, id ID, repo string) (bool, error)
}

// ProjectSetup installs toolchains and project dependencies in a synced workspace.
type ProjectSetup interface {
	SetupProjects(ctx context.Context, id ID, repoName string, steps []project.Step) error
}

// APIPublisher starts an HTTP service and routes the sandbox URL to it.
// env holds extra KEY=VALUE variables for the service process.
type APIPublisher interface {
	PublishAPI(ctx context.Context, id ID, workDir string, port int, argv, env []string) error
}

// ValidEnvKey reports whether key is a portable environment variable name:
// ASCII letters, digits, and underscores, not starting with a digit.
func ValidEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for i := range len(key) {
		c := key[i]
		letter := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_'
		digit := c >= '0' && c <= '9'
		if !letter && (i == 0 || !digit) {
			return false
		}
	}
	return true
}

// URLAuthPublic is a Sprites URL that anyone with the address can open.
const URLAuthPublic = "public"

// URLAuthSprite is a Sprites URL that requires a Sprites account.
const URLAuthSprite = "sprite"

// PublicURL changes whether a sandbox URL requires provider authentication.
type PublicURL interface {
	SetURLAuth(ctx context.Context, id ID, auth string) error
}

// Restarter reboots a sandbox in place.
type Restarter interface {
	Restart(ctx context.Context, id ID) error
}

// Upgrader moves a sandbox to the latest provider platform version.
type Upgrader interface {
	Upgrade(ctx context.Context, id ID) error
}

// RunningSupport marks a provider that can keep a sandbox up and serve a
// terminal on its URL when Spec.Running is set.
type RunningSupport interface {
	SupportsRunning()
}

// Provider is a sandbox backend (Sprites, Modal, etc.).
type Provider interface {
	Name() string
	Create(ctx context.Context, spec Spec) (ID, error)
	List(ctx context.Context) ([]Info, error)
	Inspect(ctx context.Context, id ID) (Detail, error)
	Exec(ctx context.Context, id ID, opts ExecOptions) error
	Destroy(ctx context.Context, id ID) error
}
