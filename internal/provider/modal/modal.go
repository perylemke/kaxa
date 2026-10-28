// Package modal is the Modal sandbox provider.
package modal

import (
	"context"
	"crypto/subtle"
	"fmt"
	"slices"
	"sort"
	"sync"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// Provider implements sandbox.Provider for Modal sandboxes.
//
// Modal has no in-place restart or platform upgrade, so Provider does not
// implement sandbox.Restarter or sandbox.Upgrader. A sandbox name is also its ID.
type Provider struct {
	mu          sync.Mutex
	tokenID     string
	tokenSecret string
	client      sdkClient
}

// New returns a Modal provider. Call ConfigureCredentials before use.
func New() *Provider {
	return &Provider{}
}

// NewWithClient returns a provider wired to a custom client (tests).
func NewWithClient(client sdkClient, tokenID, tokenSecret string) *Provider {
	return &Provider{client: client, tokenID: tokenID, tokenSecret: tokenSecret}
}

// ConfigureCredentials sets the Modal token ID and secret and builds the SDK
// client. Both are required, so the SDK never falls back to MODAL_TOKEN_* or
// ~/.modal.toml. Configuring the same credentials again keeps the client.
func (p *Provider) ConfigureCredentials(tokenID, tokenSecret string) error {
	if tokenID == "" || tokenSecret == "" {
		return missingCredentialsErr()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client != nil && sameCredentials(tokenID, tokenSecret, p.tokenID, p.tokenSecret) {
		return nil
	}
	c, err := newSDKClient(tokenID, tokenSecret)
	if err != nil {
		return redact(err, tokenID, tokenSecret)
	}
	if p.client != nil {
		p.client.Close()
	}
	p.tokenID, p.tokenSecret, p.client = tokenID, tokenSecret, c
	return nil
}

func sameCredentials(idA, secretA, idB, secretB string) bool {
	return subtle.ConstantTimeCompare([]byte(idA), []byte(idB))&
		subtle.ConstantTimeCompare([]byte(secretA), []byte(secretB)) == 1
}

// Name returns the registry name for Modal.
func (p *Provider) Name() string {
	return "modal"
}

// Create starts a sandbox that lives for up to 24 hours and stops after one idle hour.
func (p *Provider) Create(ctx context.Context, spec sandbox.Spec) (sandbox.ID, error) {
	if err := validateName(spec.Name); err != nil {
		return "", err
	}
	c, secrets, err := p.clientAndSecrets()
	if err != nil {
		return "", err
	}
	if _, err := c.Create(ctx, spec.Name, spec.Labels, slices.Contains(spec.Labels, sandbox.SyncLabel)); err != nil {
		return "", userErr(err, spec.Name, secrets...)
	}
	return sandbox.ID(spec.Name), nil
}

// List returns the sandboxes in the kaxa App, sorted by name.
func (p *Provider) List(ctx context.Context) ([]sandbox.Info, error) {
	c, secrets, err := p.clientAndSecrets()
	if err != nil {
		return nil, err
	}
	records, err := c.List(ctx)
	if err != nil {
		return nil, userErr(err, "", secrets...)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Name < records[j].Name })
	infos := make([]sandbox.Info, 0, len(records))
	for _, r := range records {
		infos = append(infos, sandbox.Info{
			ID:       sandbox.ID(r.Name),
			Provider: p.Name(),
			Status:   statusOf(r),
			Labels:   r.Labels,
		})
	}
	return infos, nil
}

// Inspect returns the details of one sandbox.
func (p *Provider) Inspect(ctx context.Context, id sandbox.ID) (sandbox.Detail, error) {
	if err := validateName(string(id)); err != nil {
		return sandbox.Detail{}, err
	}
	c, secrets, err := p.clientAndSecrets()
	if err != nil {
		return sandbox.Detail{}, err
	}
	r, err := c.Get(ctx, string(id))
	if err != nil {
		return sandbox.Detail{}, userErr(err, string(id), secrets...)
	}
	// Modal still resolves a name for a finished sandbox. List hides it, so
	// Inspect treats it as gone too.
	if r.ExitCode != nil {
		return sandbox.Detail{}, userErr(ErrNotFound, string(id), secrets...)
	}
	return sandbox.Detail{
		ID:       sandbox.ID(r.Name),
		Provider: p.Name(),
		Status:   statusOf(r),
		Labels:   r.Labels,
	}, nil
}

// Destroy terminates a sandbox.
func (p *Provider) Destroy(ctx context.Context, id sandbox.ID) error {
	if err := validateName(string(id)); err != nil {
		return err
	}
	c, secrets, err := p.clientAndSecrets()
	if err != nil {
		return err
	}
	return userErr(c.Terminate(ctx, string(id)), string(id), secrets...)
}

func (p *Provider) clientAndSecrets() (sdkClient, []string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client == nil {
		return nil, nil, missingCredentialsErr()
	}
	return p.client, []string{p.tokenID, p.tokenSecret}, nil
}

func statusOf(r Record) string {
	if r.ExitCode == nil {
		return "running"
	}
	return fmt.Sprintf("exited (%d)", *r.ExitCode)
}
