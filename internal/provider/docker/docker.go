// Package docker is the Docker Sandboxes provider (docker). Docker runs the
// sandboxes in its cloud.
//
// It talks to the Sandboxes REST API with a Docker ID and a personal access
// token. A sandbox is created from an image with no host mount; kaxa copies
// the repository into it. Commands run through the process API of the
// sandbox, with a short-lived endpoint credential per session.
//
// The API is experimental and needs a Docker Agentic Platform subscription.
package docker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// ProviderName is the registry name of the provider.
const ProviderName = "docker"

const (
	httpTimeout      = 60 * time.Second
	defaultPoll      = 2 * time.Second
	defaultWait      = 5 * time.Minute
	cleanupTimeout   = 30 * time.Second
	unavailableState = "unknown"
)

// Provider implements sandbox.Provider for Docker sandboxes.
//
// Docker stops a sandbox on its own after an hour without a command, and ends
// the service that PublishAPI started with it. A published port has a URL
// that Docker chooses; kaxa cannot change who may open it. A snapshot saves
// the disk and restores into a new sandbox; Docker keeps it until it is
// deleted, so kaxa names its snapshots and lists only those. Provider
// implements none of sandbox.RunningSupport, sandbox.PublicURL or
// sandbox.Upgrader. A sandbox name is its ID in kaxa, and kaxa only touches
// sandboxes and snapshots it created; a sandbox restored from a snapshot has no
// labels, so it is recorded in a local file (see adoptRestored).
type Provider struct {
	mu            sync.Mutex
	api           apiClient
	proc          processStarter
	template      string
	restored      *restoredIndex
	poll          time.Duration
	wait          time.Duration
	readyTimeout  time.Duration
	readyInterval time.Duration
}

// New returns a provider that is not yet configured; ConfigureCredentials
// connects it to the API.
func New() *Provider {
	return &Provider{poll: defaultPoll, wait: defaultWait}
}

// NewWithClients returns a provider wired to custom clients (tests).
func NewWithClients(api apiClient, proc processStarter, poll, wait time.Duration) *Provider {
	return &Provider{api: api, proc: proc, poll: poll, wait: wait}
}

// ConfigureCredentials sets the Docker ID and the personal access token.
func (p *Provider) ConfigureCredentials(id, pat string) error {
	id, pat = strings.TrimSpace(id), strings.TrimSpace(pat)
	if id == "" || pat == "" {
		return missingCredentialsErr()
	}
	hc := &http.Client{Timeout: httpTimeout}
	p.mu.Lock()
	defer p.mu.Unlock()
	rc := newRestClient(id, pat, hc)
	rc.restored = p.restoredLabels
	p.api = rc
	p.proc = &endpointClient{http: hc}
	return nil
}

// SetRestoredIndexPath sets the file that records the sandboxes restored from
// a snapshot. Without it the file is <user config dir>/kaxa/docker-restored.json.
func (p *Provider) SetRestoredIndexPath(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.restored = newRestoredIndex(path)
}

// restoredIdx returns the index of restored sandboxes, opening the default
// file the first time.
func (p *Provider) restoredIdx() (*restoredIndex, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.restored != nil {
		return p.restored, nil
	}
	path, err := defaultRestoredPath()
	if err != nil {
		return nil, err
	}
	p.restored = newRestoredIndex(path)
	return p.restored, nil
}

// restoredLabels is the labels of the restored sandboxes, by resource name.
func (p *Provider) restoredLabels() (map[string][]string, error) {
	idx, err := p.restoredIdx()
	if err != nil {
		return nil, err
	}
	return idx.labels()
}

// Name returns the registry name.
func (p *Provider) Name() string {
	return ProviderName
}

// Available reports whether credentials are configured.
func (p *Provider) Available() bool {
	api, _ := p.clients()
	return api != nil
}

func (p *Provider) clients() (apiClient, processStarter) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.api, p.proc
}

// ready returns the API client, or the error that asks for credentials.
func (p *Provider) ready() (apiClient, error) {
	api, _ := p.clients()
	if api == nil {
		return nil, missingCredentialsErr()
	}
	return api, nil
}

// fail turns an error of the API into a user-facing error, without credentials.
func (p *Provider) fail(api apiClient, err error, name string) error {
	return userErr(err, name, api.secrets()...)
}

// Create starts a sandbox from spec.Template, then the default template, then
// the image of the shell agent, and waits until it runs.
func (p *Provider) Create(ctx context.Context, spec sandbox.Spec) (sandbox.ID, error) {
	if err := validateName(spec.Name); err != nil {
		return "", err
	}
	if err := validateTemplate(spec.Template); err != nil {
		return "", err
	}
	api, err := p.ready()
	if err != nil {
		return "", err
	}
	if _, err := api.Find(ctx, spec.Name); err == nil {
		return "", p.fail(api, ErrAlreadyExists, spec.Name)
	} else if !errors.Is(err, ErrNotFound) {
		return "", p.fail(api, err, spec.Name)
	}
	image := p.templateFor(spec.Template)
	if image == "" {
		image = shellImage
	}
	rec, err := api.Create(ctx, CreateParams{Name: spec.Name, Labels: slices.Clone(spec.Labels), Image: image})
	if err != nil {
		return "", p.fail(api, err, spec.Name)
	}
	if _, err := p.waitRunning(ctx, api, rec); err != nil {
		// A sandbox that never ran would count against the quota and could
		// not be used, so it is removed.
		return "", errors.Join(p.fail(api, err, spec.Name), p.removeBest(ctx, api, rec))
	}
	return sandbox.ID(spec.Name), nil
}

// removeBest deletes a sandbox after a failed create and reports why it could not.
func (p *Provider) removeBest(ctx context.Context, api apiClient, rec Record) error {
	rmCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	if err := api.Delete(rmCtx, rec); err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("remove sandbox: %w", p.fail(api, err, rec.Name))
	}
	return nil
}

// List returns the sandboxes kaxa created, sorted by name.
func (p *Provider) List(ctx context.Context) ([]sandbox.Info, error) {
	api, err := p.ready()
	if err != nil {
		return nil, err
	}
	records, err := api.List(ctx)
	if err != nil {
		return nil, p.fail(api, err, "")
	}
	infos := make([]sandbox.Info, 0, len(records))
	for _, r := range records {
		infos = append(infos, sandbox.Info{ID: sandbox.ID(r.Name), Provider: p.Name(), Status: statusOf(r), Labels: r.Labels})
	}
	slices.SortFunc(infos, func(a, b sandbox.Info) int { return strings.Compare(string(a.ID), string(b.ID)) })
	return infos, nil
}

func statusOf(r Record) string {
	if r.Status == "" {
		return unavailableState
	}
	return r.Status
}

// Inspect returns the details of one sandbox. URL is the address of the
// published API; a sandbox without a published port has none.
func (p *Provider) Inspect(ctx context.Context, id sandbox.ID) (sandbox.Detail, error) {
	_, rec, err := p.find(ctx, id)
	if err != nil {
		return sandbox.Detail{}, err
	}
	return sandbox.Detail{
		ID:        id,
		Provider:  p.Name(),
		Status:    statusOf(rec),
		Labels:    rec.Labels,
		URL:       publishedURL(rec),
		CreatedAt: rec.CreatedAt,
	}, nil
}

// publishedURL is the address of the first published port that has one.
func publishedURL(r Record) string {
	for _, p := range r.Ports {
		if p.URL != "" {
			return p.URL
		}
	}
	return ""
}

// Destroy deletes a sandbox and everything inside it, and waits until it is gone.
func (p *Provider) Destroy(ctx context.Context, id sandbox.ID) error {
	api, rec, err := p.find(ctx, id)
	if err != nil {
		return err
	}
	if err := api.Delete(ctx, rec); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return p.fail(api, err, string(id))
	}
	if err := p.waitGone(ctx, api, rec); err != nil {
		return p.fail(api, err, string(id))
	}
	// A stale entry is harmless, because a resource name never repeats.
	_ = p.forgetRestored(rec.Resource)
	return nil
}

// find returns the sandbox kaxa created with that name. A sandbox that Docker
// has but kaxa did not create is not found: kaxa never operates on the user's
// own sandboxes.
func (p *Provider) find(ctx context.Context, id sandbox.ID) (apiClient, Record, error) {
	if err := validateName(string(id)); err != nil {
		return nil, Record{}, err
	}
	api, err := p.ready()
	if err != nil {
		return nil, Record{}, err
	}
	rec, err := api.Find(ctx, string(id))
	if err != nil {
		return nil, Record{}, p.fail(api, err, string(id))
	}
	return api, rec, nil
}

// waitFor polls a sandbox until done reports true. A sandbox that fails is an
// error with the reason Docker gave.
func (p *Provider) waitFor(ctx context.Context, api apiClient, rec Record, done func(Record) bool) (Record, error) {
	ctx, cancel := context.WithTimeout(ctx, p.wait)
	defer cancel()
	for {
		cur, err := api.Get(ctx, rec.Resource)
		if err != nil {
			return Record{}, err
		}
		if cur.Status == statusFailed {
			return Record{}, failedErr(cur)
		}
		if done(cur) {
			return cur, nil
		}
		select {
		case <-ctx.Done():
			return Record{}, fmt.Errorf("docker: sandbox %q is %s: %w", rec.Name, cur.Status, ctx.Err())
		case <-time.After(p.poll):
		}
	}
}

func failedErr(r Record) error {
	if r.Failure == "" {
		return fmt.Errorf("docker: sandbox %q failed", r.Name)
	}
	return fmt.Errorf("docker: sandbox %q failed: %s", r.Name, r.Failure)
}

func (p *Provider) waitRunning(ctx context.Context, api apiClient, rec Record) (Record, error) {
	return p.waitFor(ctx, api, rec, func(r Record) bool { return r.Status == statusRunning })
}

func (p *Provider) waitStopped(ctx context.Context, api apiClient, rec Record) (Record, error) {
	return p.waitFor(ctx, api, rec, func(r Record) bool { return r.Status == statusStopped })
}

// waitGone polls until Docker no longer knows the sandbox.
func (p *Provider) waitGone(ctx context.Context, api apiClient, rec Record) error {
	ctx, cancel := context.WithTimeout(ctx, p.wait)
	defer cancel()
	for {
		_, err := api.Get(ctx, rec.Resource)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("docker: sandbox %q is still being deleted: %w", rec.Name, ctx.Err())
		case <-time.After(p.poll):
		}
	}
}
