// Package e2b is the E2B sandbox provider.
//
// E2B has no official Go SDK, so the package talks to its REST API and to the
// daemon in each sandbox with net/http. A sandbox stops after an hour without
// a kaxa command; E2B then pauses it, and the next command resumes it.
package e2b

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

// Provider implements sandbox.Provider for E2B sandboxes.
//
// E2B has no platform upgrade, and a public sandbox URL cannot be switched
// after creation, so Provider does not implement sandbox.Upgrader,
// sandbox.PublicURL or sandbox.RunningSupport. A sandbox name is also its ID in
// kaxa; E2B's own ID is kept in the sandbox metadata.
type Provider struct {
	mu     sync.Mutex
	apiKey string
	api    apiClient
	envd   envdClient

	template                    string
	readyTimeout, readyInterval time.Duration
}

// New returns an E2B provider. Call Configure with an API key before use.
func New() *Provider {
	return &Provider{}
}

// NewWithClient returns a provider wired to custom clients (tests).
func NewWithClient(api apiClient, envd envdClient, apiKey string) *Provider {
	return &Provider{api: api, envd: envd, apiKey: apiKey}
}

// Configure sets the E2B API key and builds the clients. Configuring the same
// key again keeps the clients, so concurrent callers (such as the MCP server)
// do not replace a client that is in use.
func (p *Provider) Configure(apiKey string) error {
	if apiKey == "" {
		return missingCredentialsErr()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.api != nil && subtle.ConstantTimeCompare([]byte(apiKey), []byte(p.apiKey)) == 1 {
		return nil
	}
	p.apiKey, p.api, p.envd = apiKey, newRESTClient(apiKey), newHTTPEnvd()
	return nil
}

// SupportsTemplate marks E2B as a provider that starts sandboxes from a template.
func (*Provider) SupportsTemplate() {}

// SetDefaultTemplate sets the template used when a create names none. An empty
// name restores E2B's base template.
func (p *Provider) SetDefaultTemplate(name string) error {
	if err := validateTemplate(name); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.template = name
	return nil
}

// templateFor returns the template a create uses, or "" for the base template.
func (p *Provider) templateFor(requested string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	name := requested
	if name == "" {
		name = p.template
	}
	if name == defaultTemplate {
		return ""
	}
	return name
}

// Name returns the registry name for E2B.
func (p *Provider) Name() string {
	return "e2b"
}

// Available reports whether an API key is configured.
func (p *Provider) Available() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.api != nil
}

// Create starts a sandbox from spec.Template, then the default template, then
// E2B's base template. It runs for an hour after
// each kaxa command and is then paused, not deleted.
//
// The sandbox URLs require an access token unless the sandbox publishes an API
// (spec.ExposeAPI), since E2B cannot change that after creation.
func (p *Provider) Create(ctx context.Context, spec sandbox.Spec) (sandbox.ID, error) {
	if err := validateName(spec.Name); err != nil {
		return "", err
	}
	if err := validateTemplate(spec.Template); err != nil {
		return "", err
	}
	c, _, secrets, err := p.clients()
	if err != nil {
		return "", err
	}
	if _, err := c.Find(ctx, spec.Name); err == nil {
		return "", userErr(ErrAlreadyExists, spec.Name, secrets...)
	} else if !errors.Is(err, ErrNotFound) {
		return "", userErr(err, spec.Name, secrets...)
	}
	params := CreateParams{Name: spec.Name, Labels: spec.Labels, Template: p.templateFor(spec.Template)}
	if spec.ExposeAPI {
		params.APIPort, params.PublicTraffic = spec.APIPort, true
	}
	if _, err := c.Create(ctx, params); err != nil {
		if errors.Is(err, ErrTemplateNotFound) {
			return "", usererr.Wrap(err, fmt.Sprintf("template %q not found", params.Template),
				"check the name with the E2B CLI: e2b template list")
		}
		return "", userErr(err, spec.Name, secrets...)
	}
	return sandbox.ID(spec.Name), nil
}

// List returns the sandboxes kaxa created, sorted by name.
func (p *Provider) List(ctx context.Context) ([]sandbox.Info, error) {
	c, _, secrets, err := p.clients()
	if err != nil {
		return nil, err
	}
	records, err := c.List(ctx)
	if err != nil {
		return nil, userErr(err, "", secrets...)
	}
	slices.SortFunc(records, func(a, b Record) int {
		switch {
		case a.Name < b.Name:
			return -1
		case a.Name > b.Name:
			return 1
		}
		return 0
	})
	infos := make([]sandbox.Info, 0, len(records))
	for _, r := range records {
		infos = append(infos, sandbox.Info{
			ID:       sandbox.ID(r.Name),
			Provider: p.Name(),
			Status:   r.State,
			Labels:   r.Labels,
		})
	}
	return infos, nil
}

// Inspect returns the details of one sandbox. URL is the address of the
// published API; a sandbox without one has none.
func (p *Provider) Inspect(ctx context.Context, id sandbox.ID) (sandbox.Detail, error) {
	if err := validateName(string(id)); err != nil {
		return sandbox.Detail{}, err
	}
	c, _, secrets, err := p.clients()
	if err != nil {
		return sandbox.Detail{}, err
	}
	rec, err := c.Find(ctx, string(id))
	if err != nil {
		return sandbox.Detail{}, userErr(err, string(id), secrets...)
	}
	d := sandbox.Detail{
		ID:        sandbox.ID(rec.Name),
		Provider:  p.Name(),
		Status:    rec.State,
		Labels:    rec.Labels,
		CreatedAt: rec.StartedAt,
	}
	if rec.APIPort > 0 {
		d.URL, d.Auth = "https://"+strconv.Itoa(rec.APIPort)+"-"+rec.ID+"."+defaultDomain, sandbox.URLAuthPublic
	}
	return d, nil
}

// Destroy kills a sandbox, running or paused.
func (p *Provider) Destroy(ctx context.Context, id sandbox.ID) error {
	if err := validateName(string(id)); err != nil {
		return err
	}
	c, _, secrets, err := p.clients()
	if err != nil {
		return err
	}
	rec, err := c.Find(ctx, string(id))
	if err != nil {
		return userErr(err, string(id), secrets...)
	}
	return userErr(c.Kill(ctx, rec.ID), string(id), secrets...)
}

// clients returns the configured clients and the secrets to keep out of errors.
func (p *Provider) clients() (apiClient, envdClient, []string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.api == nil {
		return nil, nil, nil, missingCredentialsErr()
	}
	return p.api, p.envd, []string{p.apiKey}, nil
}
