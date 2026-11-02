// Package blaxel is the Blaxel sandbox provider.
//
// It uses the official Blaxel Go SDK (github.com/blaxel-ai/sdk-go) behind the
// apiClient and runtime interfaces, so tests run on fakes. Sandboxes scale to
// zero when idle and resume with their files and processes. kaxa does not
// restart, snapshot, or open a terminal: the process API has no PTY.
package blaxel

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
	ProviderName = "blaxel"

	defaultImage    = "blaxel/base-image:latest"
	defaultRegion   = "us-pdx-1"
	defaultMemory   = 4096
	defaultTimeout  = 3 * time.Minute
	defaultPoll     = time.Second
	cleanupTimeout  = 15 * time.Second
	controlPlaneURL = "https://api.blaxel.ai/v0"
)

// Provider implements sandbox.Provider for Blaxel sandboxes.
//
// Blaxel has no in-place restart, no snapshot in this version, and no terminal
// on the sandbox URL, so Provider does not implement sandbox.Restarter,
// sandbox.Upgrader, sandbox.PublicURL, sandbox.RunningSupport, or sandbox.Snapshotter.
type Provider struct {
	mu        sync.Mutex
	apiKey    string
	workspace string
	api       apiClient
	rt        runtime
	// recent holds sandboxes this process created until they are deleted. Find
	// can miss one whose labels are not visible yet.
	recent sync.Map

	template string
	region   string
	memory   int
	// startTimeout and pollInterval bound the waits for a sandbox.
	startTimeout, pollInterval time.Duration
	readyTimeout, readyEvery   time.Duration
}

// New returns a Blaxel provider. Call Configure before use.
func New() *Provider {
	return &Provider{}
}

// NewWithClients returns a provider wired to custom clients (tests).
func NewWithClients(api apiClient, rt runtime, apiKey, workspace string) *Provider {
	return &Provider{api: api, rt: rt, apiKey: apiKey, workspace: workspace}
}

// Configure sets the API key and workspace and builds the clients. Configuring
// the same pair again keeps the clients, so concurrent callers do not replace
// a client that is in use.
func (p *Provider) Configure(apiKey, workspace string) error {
	if apiKey == "" || workspace == "" {
		return missingCredentialsErr()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	sameKey := subtle.ConstantTimeCompare([]byte(apiKey), []byte(p.apiKey)) == 1
	sameSpace := subtle.ConstantTimeCompare([]byte(workspace), []byte(p.workspace)) == 1
	if p.api != nil && sameKey && sameSpace {
		return nil
	}
	c := newSDKClient(apiKey, workspace)
	p.apiKey, p.workspace, p.api, p.rt = apiKey, workspace, c, c
	return nil
}

// SupportsTemplate marks Blaxel as a provider that starts sandboxes from an image.
func (*Provider) SupportsTemplate() {}

// SetDefaultTemplate sets the image used when a create names none. An empty
// name restores blaxel/base-image:latest.
func (p *Provider) SetDefaultTemplate(name string) error {
	if err := validateTemplate(name); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.template = name
	return nil
}

// SetRegion sets the region of a new sandbox. An empty region restores us-pdx-1.
func (p *Provider) SetRegion(region string) error {
	if err := validateRegion(region); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.region = region
	return nil
}

// SetMemory sets the memory of a new sandbox, in megabytes. Zero restores 4096.
func (p *Provider) SetMemory(mb int) error {
	if err := validateMemory(mb); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.memory = mb
	return nil
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
		timeout = defaultTimeout
	}
	if interval <= 0 {
		interval = defaultPoll
	}
	return timeout, interval
}

// Name returns the registry name for Blaxel.
func (p *Provider) Name() string { return ProviderName }

// Available reports whether an API key and a workspace are configured.
func (p *Provider) Available() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.api != nil
}

// Create starts a sandbox and waits until it can run commands.
func (p *Provider) Create(ctx context.Context, spec sandbox.Spec) (sandbox.ID, error) {
	if err := validateName(spec.Name); err != nil {
		return "", err
	}
	params, err := p.createParams(spec)
	if err != nil {
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
	rec, err := c.Create(ctx, params)
	if err != nil {
		return "", userErr(err, spec.Name, secrets...)
	}
	rec, err = p.waitRunning(ctx, c, rec)
	if err != nil {
		return "", errors.Join(userErr(err, spec.Name, secrets...), removeBest(ctx, c, spec.Name))
	}
	p.recent.Store(spec.Name, rec)
	return sandbox.ID(spec.Name), nil
}

func (p *Provider) createParams(spec sandbox.Spec) (CreateParams, error) {
	image, err := p.imageFor(spec.Template)
	if err != nil {
		return CreateParams{}, err
	}
	params := CreateParams{
		Name: spec.Name, Image: image, Region: p.regionFor(), Memory: p.memoryFor(), Labels: spec.Labels,
	}
	if !spec.ExposeAPI {
		return params, nil
	}
	if err := validateAPIPort(spec.APIPort); err != nil {
		return CreateParams{}, err
	}
	params.APIPort = spec.APIPort
	return params, nil
}

func (p *Provider) imageFor(requested string) (string, error) {
	if err := validateTemplate(requested); err != nil {
		return "", err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if requested != "" {
		return requested, nil
	}
	if p.template != "" {
		return p.template, nil
	}
	return defaultImage, nil
}

func (p *Provider) regionFor() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.region != "" {
		return p.region
	}
	return defaultRegion
}

func (p *Provider) memoryFor() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.memory > 0 {
		return p.memory
	}
	return defaultMemory
}

func removeBest(ctx context.Context, c apiClient, name string) error {
	rmCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	if err := c.Delete(rmCtx, name); err != nil && !errors.Is(err, ErrNotFound) {
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

// Inspect returns the details of one sandbox. URL is the public preview of the
// published API; a sandbox without one has none.
func (p *Provider) Inspect(ctx context.Context, id sandbox.ID) (sandbox.Detail, error) {
	if err := validateName(string(id)); err != nil {
		return sandbox.Detail{}, err
	}
	c, _, secrets, err := p.clients()
	if err != nil {
		return sandbox.Detail{}, err
	}
	rec, err := p.resolve(ctx, c, string(id))
	if err != nil {
		return sandbox.Detail{}, userErr(err, string(id), secrets...)
	}
	d := sandbox.Detail{
		ID: sandbox.ID(rec.Name), Provider: p.Name(), Status: rec.Status, Labels: rec.Labels, CreatedAt: rec.CreatedAt,
	}
	if rec.APIPort == 0 {
		return d, nil
	}
	url, err := c.PreviewURL(ctx, rec.Name)
	switch {
	case err == nil:
		d.URL, d.Auth = url, sandbox.URLAuthPublic
	case !errors.Is(err, ErrNotFound):
		return sandbox.Detail{}, userErr(err, string(id), secrets...)
	}
	return d, nil
}

// Destroy deletes a sandbox.
func (p *Provider) Destroy(ctx context.Context, id sandbox.ID) error {
	if err := validateName(string(id)); err != nil {
		return err
	}
	c, _, secrets, err := p.clients()
	if err != nil {
		return err
	}
	if _, err := p.resolve(ctx, c, string(id)); err != nil {
		return userErr(err, string(id), secrets...)
	}
	if err := c.Delete(ctx, string(id)); err != nil {
		return userErr(err, string(id), secrets...)
	}
	p.recent.Delete(string(id))
	return nil
}

func (p *Provider) resolve(ctx context.Context, c apiClient, name string) (Record, error) {
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

func (p *Provider) clients() (apiClient, runtime, []string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.api == nil || p.rt == nil {
		return nil, nil, nil, missingCredentialsErr()
	}
	return p.api, p.rt, []string{p.apiKey}, nil
}

func (p *Provider) waitRunning(ctx context.Context, c apiClient, rec Record) (Record, error) {
	timeout, interval := p.polling()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		switch rec.Status {
		case statusRunning:
			return rec, nil
		case statusError, statusArchived, statusStopped:
			return Record{}, usererr.New(
				fmt.Sprintf("sandbox %q is %s", rec.Name, rec.Status),
				"destroy it and create a new one; the Blaxel console shows the cause",
			)
		}
		if err := sleep(ctx, interval); err != nil {
			return Record{}, fmt.Errorf("blaxel: sandbox %q is %s: %w", rec.Name, rec.Status, err)
		}
		cur, err := c.Find(ctx, rec.Name)
		if err != nil {
			return Record{}, err
		}
		rec = cur
	}
}

func (p *Provider) open(ctx context.Context, id sandbox.ID) (Record, []string, error) {
	if err := validateName(string(id)); err != nil {
		return Record{}, nil, err
	}
	c, _, secrets, err := p.clients()
	if err != nil {
		return Record{}, nil, err
	}
	rec, err := p.resolve(ctx, c, string(id))
	if err != nil {
		return Record{}, secrets, userErr(err, string(id), secrets...)
	}
	if rec.Status == statusStarting {
		rec, err = p.waitRunning(ctx, c, rec)
		if err != nil {
			return Record{}, secrets, userErr(err, string(id), secrets...)
		}
	}
	if err := usable(rec); err != nil {
		return Record{}, secrets, err
	}
	return rec, secrets, nil
}

func usable(rec Record) error {
	switch rec.Status {
	case statusRunning:
		return nil
	case statusArchived:
		return usererr.New(
			fmt.Sprintf("sandbox %q is archived", rec.Name),
			"unarchive it in the Blaxel console, or create a new sandbox",
		)
	default:
		return usererr.New(
			fmt.Sprintf("sandbox %q is %s", rec.Name, rec.Status),
			"destroy it and create a new one; the Blaxel console shows the cause",
		)
	}
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

func (p *Provider) synced(rec Record) bool {
	return slices.Contains(rec.Labels, sandbox.SyncLabel)
}
