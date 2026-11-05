// Package daytona is the Daytona sandbox provider.
//
// It uses the official Daytona Go SDK (github.com/daytona/clients/sdk-go)
// behind the apiClient and toolboxClient interfaces, so tests run on fakes.
// Sandboxes are persistent: after the idle timeout Daytona stops them and keeps the disk,
// and the next kaxa command starts them again.
package daytona

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	// ProviderName is the registry name of the provider.
	ProviderName = "daytona"

	defaultStartTimeout = 3 * time.Minute
	defaultPollInterval = time.Second
	cleanupTimeout      = 15 * time.Second
)

// Provider implements sandbox.Provider for Daytona sandboxes.
//
// Daytona has no platform upgrade and no terminal on the sandbox URL, so
// Provider does not implement sandbox.Upgrader, sandbox.PublicURL or
// sandbox.RunningSupport. A sandbox name is also its ID in kaxa; Daytona's own
// ID is kept inside the provider.
type Provider struct {
	mu     sync.Mutex
	apiKey string
	api    apiClient
	tb     toolboxClient
	// recent holds sandboxes this process created until they are deleted. Find
	// can miss one whose labels are not visible yet; sync and destroy then use
	// the Daytona id from here.
	recent sync.Map

	template string
	// startTimeout and pollInterval bound the waits for a sandbox or an image.
	startTimeout, pollInterval time.Duration
	readyTimeout, readyEvery   time.Duration
	snapshotIndex              string
}

// New returns a Daytona provider. Call Configure with an API key before use.
func New() *Provider {
	return &Provider{}
}

// NewWithClients returns a provider wired to custom clients (tests).
func NewWithClients(api apiClient, tb toolboxClient, apiKey string) *Provider {
	return &Provider{api: api, tb: tb, apiKey: apiKey}
}

// Configure sets the Daytona API key and builds the clients. Configuring the
// same key again keeps the clients, so concurrent callers (such as the MCP
// server) do not replace a client that is in use.
func (p *Provider) Configure(apiKey string) error {
	if apiKey == "" {
		return missingCredentialsErr()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.api != nil && subtle.ConstantTimeCompare([]byte(apiKey), []byte(p.apiKey)) == 1 {
		return nil
	}
	c, err := newSDKClient(apiKey)
	if err != nil {
		return userErr(err, "", apiKey)
	}
	p.apiKey, p.api, p.tb = apiKey, c, &sdkToolbox{c: c}
	return nil
}

// SupportsTemplate marks Daytona as a provider that starts sandboxes from an image.
func (*Provider) SupportsTemplate() {}

// SetDefaultTemplate sets the image used when a create names none. An empty
// name restores the platform default image.
func (p *Provider) SetDefaultTemplate(name string) error {
	if err := validateTemplate(name); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.template = name
	return nil
}

func (p *Provider) templateFor(requested string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if requested != "" {
		return requested
	}
	return p.template
}

// SetPolling sets how long a sandbox may take to start and how often its
// status is read. Tests use it to avoid long waits.
func (p *Provider) SetPolling(timeout, interval time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.startTimeout, p.pollInterval = timeout, interval
}

func (p *Provider) polling() (timeout, interval time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	timeout, interval = p.startTimeout, p.pollInterval
	if timeout <= 0 {
		timeout = defaultStartTimeout
	}
	if interval <= 0 {
		interval = defaultPollInterval
	}
	return timeout, interval
}

// Name returns the registry name for Daytona.
func (p *Provider) Name() string {
	return ProviderName
}

// Available reports whether an API key is configured.
func (p *Provider) Available() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.api != nil
}

// Create starts a persistent sandbox from spec.Template, then the default
// image, then Daytona's platform image, and waits until it runs.
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
		params.APIPort = spec.APIPort
	}
	rec, err := c.Create(ctx, params)
	if err != nil {
		return "", userErr(err, spec.Name, secrets...)
	}
	created := rec
	rec, err = p.waitRunning(ctx, c, rec)
	if err != nil {
		return "", errors.Join(userErr(err, spec.Name, secrets...), removeBest(ctx, c, created.ID))
	}
	p.rememberRecord(spec.Name, rec)
	return sandbox.ID(spec.Name), nil
}

// removeBest deletes a sandbox after a failed create and reports why it could not.
func removeBest(ctx context.Context, c apiClient, id string) error {
	rmCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	if err := c.Delete(rmCtx, id); err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("remove sandbox: %w", err)
	}
	return nil
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
	slices.SortFunc(records, func(a, b Record) int { return strings.Compare(a.Name, b.Name) })
	infos := make([]sandbox.Info, 0, len(records))
	for _, r := range records {
		infos = append(infos, sandbox.Info{ID: sandbox.ID(r.Name), Provider: p.Name(), Status: r.Status, Labels: r.Labels})
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
	rec, err := p.resolveRecord(ctx, c, string(id))
	if err != nil {
		return sandbox.Detail{}, userErr(err, string(id), secrets...)
	}
	full, err := c.Get(ctx, rec.ID)
	if err == nil {
		rec = full
	}
	d := sandbox.Detail{
		ID: sandbox.ID(rec.Name), Provider: p.Name(), Status: rec.Status, Labels: rec.Labels, CreatedAt: rec.CreatedAt,
	}
	var url, auth string
	if rec.APIPort > 0 {
		ep, err := c.GetPort(ctx, rec.ID, rec.APIPort)
		switch {
		case err == nil && ep.Public:
			url, auth = ep.URL, sandbox.URLAuthPublic
		case err != nil && !errors.Is(err, ErrNotFound):
			return sandbox.Detail{}, userErr(err, string(id), secrets...)
		}
	}
	d.URL, d.Auth = url, auth
	d.Fields = daytonaDetailFields(rec, url, auth)
	return d, nil
}

// Destroy deletes a sandbox, running or stopped.
func (p *Provider) Destroy(ctx context.Context, id sandbox.ID) error {
	if err := validateName(string(id)); err != nil {
		return err
	}
	c, _, secrets, err := p.clients()
	if err != nil {
		return err
	}
	rec, err := p.resolveRecord(ctx, c, string(id))
	if err != nil {
		return userErr(err, string(id), secrets...)
	}
	if err := c.Delete(ctx, rec.ID); err != nil {
		return userErr(err, string(id), secrets...)
	}
	p.forgetRecord(string(id))
	return nil
}

func (p *Provider) rememberRecord(name string, rec Record) {
	p.recent.Store(name, rec)
}

func (p *Provider) forgetRecord(name string) {
	p.recent.Delete(name)
}

// resolveRecord finds a sandbox by kaxa name. When the API read misses labels
// that are not visible yet, it falls back to the record from Create in this process.
func (p *Provider) resolveRecord(ctx context.Context, c apiClient, name string) (Record, error) {
	rec, err := c.Find(ctx, name)
	if err == nil {
		return rec, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Record{}, err
	}
	if v, ok := p.recent.Load(name); ok {
		if stored, ok := v.(Record); ok {
			return stored, nil
		}
	}
	return Record{}, ErrNotFound
}

// clients returns the configured clients and the secrets to keep out of errors.
func (p *Provider) clients() (apiClient, toolboxClient, []string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.api == nil {
		return nil, nil, nil, missingCredentialsErr()
	}
	return p.api, p.tb, []string{p.apiKey}, nil
}

// waitRunning polls a sandbox until it runs.
func (p *Provider) waitRunning(ctx context.Context, c apiClient, rec Record) (Record, error) {
	return p.waitStatus(ctx, c, rec, statusRunning)
}

// waitStatus polls a sandbox until it reaches target. A sandbox that failed or
// was lost never will, so that ends the wait at once.
func (p *Provider) waitStatus(ctx context.Context, c apiClient, rec Record, target string) (Record, error) {
	timeout, interval := p.polling()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		switch rec.Status {
		case target:
			return rec, nil
		case statusFailed, "lost", statusError:
			return Record{}, usererr.New(
				fmt.Sprintf("sandbox %q is %s", rec.Name, rec.Status),
				"destroy it and create a new one; the Daytona dashboard shows the cause")
		}
		if err := sleep(ctx, interval); err != nil {
			return Record{}, fmt.Errorf("daytona: sandbox %q is %s: %w", rec.Name, rec.Status, err)
		}
		cur, err := c.Get(ctx, rec.ID)
		if err != nil {
			return Record{}, err
		}
		rec = cur
	}
}

// ensureRunning starts a stopped sandbox and resumes a paused one, then waits.
func (p *Provider) ensureRunning(ctx context.Context, c apiClient, rec Record) (Record, error) {
	switch rec.Status {
	case statusStopped:
		if err := c.Start(ctx, rec.ID); err != nil {
			return Record{}, err
		}
	case statusPaused:
		if err := c.Resume(ctx, rec.ID); err != nil {
			return Record{}, err
		}
	}
	return p.waitRunning(ctx, c, rec)
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
