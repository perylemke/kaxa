// Package vercel is the Vercel Sandbox provider.
//
// Vercel has no Go SDK, so the package talks to the REST API with net/http.
// A sandbox is named and persistent: stopping its session snapshots the
// filesystem, and the next command resumes the name from that state. Files
// survive a stop; running processes do not.
package vercel

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

// ProviderName is the registry name of the provider.
const ProviderName = "vercel"

// Provider implements sandbox.Provider for Vercel Sandboxes.
//
// The REST API has no terminal, so Exec rejects TTY. Memory follows the vCPU
// count, routes are public, and there is no restart-in-place upgrade, so
// Provider does not implement sandbox.MemorySupport, sandbox.PublicURL,
// sandbox.RunningSupport, or sandbox.Upgrader.
type Provider struct {
	mu                          sync.Mutex
	token, project, team        string
	api                         apiClient
	template                    string
	snapshotIndex               string
	readyTimeout, readyInterval time.Duration
}

// New returns a Vercel provider. Call Configure before use.
func New() *Provider { return &Provider{} }

// NewWithClient returns a provider wired to a custom client (tests).
func NewWithClient(api apiClient, token string) *Provider {
	return &Provider{api: api, token: token, project: "test"}
}

// NewWithBaseURL returns a provider that calls base instead of api.vercel.com.
// The token only goes to an HTTPS URL or a loopback address, so tests can use
// httptest and nothing else can redirect it.
func NewWithBaseURL(base, token, project, team string) (*Provider, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("vercel: invalid base URL %q", base)
	}
	if u.Scheme != "https" && !isLoopback(u.Hostname()) {
		return nil, errors.New("vercel: the token is only sent to HTTPS URLs or loopback")
	}
	return &Provider{
		token: token, project: project, team: team,
		api: newRESTClient(base, token, project, team),
	}, nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Configure sets the token, project, and team and builds the client.
// Configuring the same values again keeps the client, so concurrent callers
// (such as the MCP server) do not replace a client that is in use.
func (p *Provider) Configure(token, project, team string) error {
	if token == "" || project == "" {
		return missingCredentialsErr()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.api != nil && subtle.ConstantTimeCompare([]byte(token), []byte(p.token)) == 1 &&
		project == p.project && team == p.team {
		return nil
	}
	p.token, p.project, p.team = token, project, team
	p.api = newRESTClient(defaultBaseURL, token, project, team)
	return nil
}

// SupportsTemplate marks Vercel as a provider that starts sandboxes from an image.
func (*Provider) SupportsTemplate() {}

// SupportsCPU marks Vercel as a provider that honors Spec.CPU.
func (*Provider) SupportsCPU() {}

// ComputeOptions lists the vCPU choices Vercel offers a form. Memory follows
// the vCPUs, so only the CPU is a choice.
func (*Provider) ComputeOptions() sandbox.ComputeOptions {
	return sandbox.ComputeOptions{CPUs: slices.Clone(sandbox.VercelCPUs), CustomCPU: true}
}

// SupportsNetworkBlock marks Vercel as a provider that can deny outbound traffic.
func (*Provider) SupportsNetworkBlock() {}

// SetDefaultTemplate sets the image used when a create names none.
func (p *Provider) SetDefaultTemplate(name string) error {
	if err := validateTemplate(name); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.template = name
	return nil
}

func (p *Provider) imageFor(requested string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if requested != "" {
		return requested
	}
	return p.template
}

// Name returns the registry name for Vercel.
func (p *Provider) Name() string { return ProviderName }

// Available reports whether a token and project are configured.
func (p *Provider) Available() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.api != nil
}

// clients returns the configured client and the secrets to keep out of errors.
func (p *Provider) clients() (apiClient, []string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.api == nil {
		return nil, nil, missingCredentialsErr()
	}
	return p.api, []string{p.token}, nil
}

// Create starts a persistent sandbox from spec.Template, then the default
// image, then the Vercel default. Its session runs for 30 minutes and kaxa
// extends it on every command.
func (p *Provider) Create(ctx context.Context, spec sandbox.Spec) (sandbox.ID, error) {
	if err := validateName(spec.Name); err != nil {
		return "", err
	}
	if err := validateTemplate(spec.Template); err != nil {
		return "", err
	}
	vcpus, err := vcpusFor(spec.CPU)
	if err != nil {
		return "", err
	}
	if spec.ExposeAPI && (spec.APIPort < 1024 || spec.APIPort > 65535 || spec.APIPort == reservedPort) {
		return "", usererr.New(
			fmt.Sprintf("invalid api port %d for vercel", spec.APIPort),
			fmt.Sprintf("use a port from 1024 to 65535 other than %d", reservedPort),
		)
	}
	env, err := envMap(spec.Env)
	if err != nil {
		return "", err
	}
	c, secrets, err := p.clients()
	if err != nil {
		return "", err
	}
	params := CreateParams{
		Name: spec.Name, Labels: spec.Labels, Image: p.imageFor(spec.Template),
		VCPUs: vcpus, Env: env, BlockNetwork: spec.NetworkBlockAll,
	}
	if spec.ExposeAPI {
		params.APIPort = spec.APIPort
	}
	if _, err := c.Create(ctx, params); err != nil {
		return "", userErr(err, spec.Name, secrets...)
	}
	return sandbox.ID(spec.Name), nil
}

// reservedPort is the port Vercel keeps for its own use.
const reservedPort = 23456

func envMap(entries []string) (map[string]string, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		key, value, ok := cutEnv(e)
		if !ok || !sandbox.ValidEnvKey(key) {
			return nil, usererr.New("invalid env entry", "use KEY=VALUE")
		}
		out[key] = value
	}
	return out, nil
}

// List returns the sandboxes kaxa created, sorted by name.
func (p *Provider) List(ctx context.Context) ([]sandbox.Info, error) {
	c, secrets, err := p.clients()
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
		infos = append(infos, sandbox.Info{ID: sandbox.ID(r.Name), Provider: p.Name(), Status: r.Status, Labels: r.Labels})
	}
	return infos, nil
}

// Inspect returns the details of one sandbox without resuming it. URL is the
// public address of the published API; a sandbox without one has none.
func (p *Provider) Inspect(ctx context.Context, id sandbox.ID) (sandbox.Detail, error) {
	if err := validateName(string(id)); err != nil {
		return sandbox.Detail{}, err
	}
	c, secrets, err := p.clients()
	if err != nil {
		return sandbox.Detail{}, err
	}
	rec, err := c.Get(ctx, string(id), false)
	if err != nil {
		return sandbox.Detail{}, userErr(err, string(id), secrets...)
	}
	d := sandbox.Detail{
		ID: sandbox.ID(rec.Name), Provider: p.Name(), Status: rec.Status, Labels: rec.Labels,
		CreatedAt: rec.CreatedAt,
	}
	if link := apiURL(rec); link != "" {
		d.URL, d.Auth = link, sandbox.URLAuthPublic
	}
	d.Fields = vercelDetailFields(rec, d.URL, d.Auth)
	return d, nil
}

// apiURL is the route of the published API port, or "".
func apiURL(rec Record) string {
	if rec.APIPort == 0 {
		return ""
	}
	for _, r := range rec.Routes {
		if r.Port == rec.APIPort {
			return r.URL
		}
	}
	return ""
}

// Destroy deletes a sandbox, its snapshots, and the comments kaxa kept for them.
func (p *Provider) Destroy(ctx context.Context, id sandbox.ID) error {
	if err := validateName(string(id)); err != nil {
		return err
	}
	c, secrets, err := p.clients()
	if err != nil {
		return err
	}
	// Reading first makes sure the sandbox is one kaxa created.
	if _, err := c.Get(ctx, string(id), false); err != nil {
		return userErr(err, string(id), secrets...)
	}
	if err := c.Delete(ctx, string(id)); err != nil {
		return userErr(err, string(id), secrets...)
	}
	return p.forgetSandboxSnapshots(string(id))
}
