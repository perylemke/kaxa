package blaxel

import (
	"context"
	"io"
	"time"

	sdk "github.com/blaxel-ai/sdk-go"
)

const (
	labelManaged  = "kaxa.managed"
	labelName     = "kaxa.name"
	labelLabels   = "kaxa.labels"
	labelAPIPort  = "kaxa.api_port"
	labelTemplate = "kaxa.template"
	labelTrue     = "true"

	statusRunning  = "running"
	statusStarting = "starting"
	statusError    = "error"
	statusArchived = "archived"
	statusStopped  = "stopped"

	// Blaxel keeps these on Get after the sandbox is finished. The name can be reused.
	phaseFailed     = "FAILED"
	phaseTerminated = "TERMINATED"
	// phaseDeleting still holds the name until the sandbox is terminated.
	phaseDeleting = "DELETING"

	previewName = "kaxa-api"
)

// CreateParams describes the sandbox to create.
type CreateParams struct {
	Name    string
	Image   string
	Region  string
	Memory  int
	Labels  []string
	APIPort int
}

// Record is one Blaxel sandbox as kaxa sees it. The kaxa name is the Blaxel name.
type Record struct {
	Name   string
	Status string
	// Phase is the API status. DELETING and TERMINATED both display as stopped,
	// and only the raw value says whether the name is free.
	Phase      string
	Labels     []string
	Image      string
	Region     string
	APIPort    int
	PreviewURL string
	CreatedAt  time.Time
}

// runSpec is one command inside a sandbox.
type runSpec struct {
	argv           []string
	dir            string
	env            []string
	stdin          io.Reader
	stdout, stderr io.Writer
	// port, when set, makes Start return once that port accepts connections.
	port int
}

// apiClient is the control plane: sandboxes and previews.
type apiClient interface {
	Create(ctx context.Context, p CreateParams) (Record, error)
	List(ctx context.Context) ([]Record, error)
	Find(ctx context.Context, name string) (Record, error)
	// Sandbox returns the sandbox document for inspect detail fields.
	Sandbox(ctx context.Context, name string) (*sdk.Sandbox, error)
	Delete(ctx context.Context, name string) error
	// PreviewURL returns the public preview URL, or ErrNotFound when none exists.
	PreviewURL(ctx context.Context, name string) (string, error)
	CreatePreview(ctx context.Context, name string, port int) (string, error)
	// ListHub returns the pre-built sandbox templates from the Blaxel Hub.
	ListHub(ctx context.Context) ([]hubTemplate, error)
}

// hubTemplate is one image the Blaxel Hub offers for a new sandbox.
type hubTemplate struct {
	Image       string
	DisplayName string
	// Memory is the hub's suggested size, in MB. Zero means the hub did not set one.
	Memory     int
	Hidden     bool
	ComingSoon bool
}

// runtime runs commands and moves files on the sandbox data plane.
type runtime interface {
	// Run waits until the command exits and returns its exit code.
	Run(ctx context.Context, name string, spec runSpec) (int, error)
	// Start launches a long-running command and returns once spec.port is open,
	// or once the command exits.
	Start(ctx context.Context, name string, spec runSpec) error
	// Terminal attaches to the sandbox terminal. The sandbox starts the shell.
	Terminal(ctx context.Context, name string, spec termSpec) error
	Upload(ctx context.Context, name, path string, data []byte) error
	Remove(ctx context.Context, name, path string) error
}
