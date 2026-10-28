// Package sprites is the Fly.io Sprites sandbox provider.
package sprites

import (
	"context"
	"crypto/subtle"
	"errors"
	"sync"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// Provider implements sandbox.Provider for Fly.io Sprites.
type Provider struct {
	mu     sync.Mutex
	token  string
	client sdkClient
}

// New returns a Sprites provider. Call Configure with a token before use.
func New() *Provider {
	return &Provider{}
}

// Configure sets the Sprites API token and builds the SDK client.
// Configuring the same token again keeps the existing client, so concurrent
// callers (such as the MCP server) do not close a client that is in use.
func (p *Provider) Configure(token string) error {
	if token == "" {
		return missingTokenErr()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client != nil && subtle.ConstantTimeCompare([]byte(token), []byte(p.token)) == 1 {
		return nil
	}
	if p.client != nil {
		_ = p.client.Close()
	}
	p.token = token
	p.client = newSDKClient(token, defaultAPIBase)
	return nil
}

// Name returns the registry name for Sprites.
func (p *Provider) Name() string {
	return "sprites"
}

// SupportsRunning marks that Sprites can keep a sandbox up and serve tmux.
func (p *Provider) SupportsRunning() {}

// Create provisions a new sprite.
func (p *Provider) Create(ctx context.Context, spec sandbox.Spec) (sandbox.ID, error) {
	if err := validateName(spec.Name); err != nil {
		return "", err
	}
	c, token, err := p.clientAndToken()
	if err != nil {
		return "", err
	}
	sp, err := c.CreateSprite(ctx, spec.Name, spec.Labels)
	if err != nil {
		return "", redactErr(err, token)
	}
	name := sp.Name()
	if err := waitForWarm(ctx, c, name, token); err != nil {
		return "", err
	}
	if spec.Running {
		if err := configureRunning(ctx, c, name, token, spec.ExposeAPI); err != nil {
			return "", redactErr(err, token)
		}
	}
	return sandbox.ID(name), nil
}

// List returns all sprites in the organization.
func (p *Provider) List(ctx context.Context) ([]sandbox.Info, error) {
	c, token, err := p.clientAndToken()
	if err != nil {
		return nil, err
	}
	entries, err := c.ListEntries(ctx, "")
	if err != nil {
		return nil, redactErr(err, token)
	}
	return entriesToSandboxInfo(entries, p.Name()), nil
}

// Upgrade upgrades a sprite to the latest platform version.
func (p *Provider) Upgrade(ctx context.Context, id sandbox.ID) error {
	if err := validateName(string(id)); err != nil {
		return err
	}
	c, token, err := p.clientAndToken()
	if err != nil {
		return err
	}
	if err := c.UpgradeSprite(ctx, string(id)); err != nil {
		return redactErr(err, token)
	}
	return nil
}

// Exec runs a command or interactive shell on a sprite.
func (p *Provider) Exec(ctx context.Context, id sandbox.ID, opts sandbox.ExecOptions) error {
	if err := validateName(string(id)); err != nil {
		return err
	}
	if len(opts.Command) == 0 {
		return errors.New("sprites exec: empty command")
	}
	c, token, err := p.clientAndToken()
	if err != nil {
		return err
	}
	sp, err := c.GetSprite(ctx, string(id))
	if err != nil {
		return redactErr(err, token)
	}

	cmd, err := prepareExecCmd(ctx, c.Sprite(string(id)), opts, spriteLabels(sp))
	if err != nil {
		return redactErr(err, token)
	}
	if opts.TTY && opts.Resize != nil {
		go p.watchResize(ctx, cmd, opts.Resize)
	}

	if err := cmd.Run(); err != nil {
		return mapExecErr(err, token)
	}
	return nil
}

// Destroy deletes a sprite.
func (p *Provider) Destroy(ctx context.Context, id sandbox.ID) error {
	if err := validateName(string(id)); err != nil {
		return err
	}
	c, token, err := p.clientAndToken()
	if err != nil {
		return err
	}
	if err := c.DestroySprite(ctx, string(id)); err != nil {
		return redactErr(err, token)
	}
	return nil
}

func prepareExecCmd(ctx context.Context, sprite *spritesdk.Sprite, opts sandbox.ExecOptions, labels []string) (*spritesdk.Cmd, error) {
	cmd := sprite.CommandContext(ctx, opts.Command[0], opts.Command[1:]...)
	cmd.Dir = workspaceExecDir(labels)
	if opts.TTY {
		cmd.SetTTY(true)
		if opts.Size.Rows > 0 && opts.Size.Cols > 0 {
			if err := cmd.SetTTYSize(opts.Size.Rows, opts.Size.Cols); err != nil {
				return nil, err
			}
		}
	}
	cmd.Stdin = opts.Stdin
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	return cmd, nil
}

func (p *Provider) clientAndToken() (sdkClient, string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client == nil {
		return nil, "", missingTokenErr()
	}
	return p.client, p.token, nil
}

func (p *Provider) watchResize(ctx context.Context, cmd *spritesdk.Cmd, resize <-chan sandbox.TermSize) {
	for {
		select {
		case <-ctx.Done():
			return
		case size, ok := <-resize:
			if !ok {
				return
			}
			_ = cmd.SetTTYSize(size.Rows, size.Cols)
		}
	}
}

// NewWithClient returns a provider wired to a custom client (tests).
func NewWithClient(client sdkClient, token string) *Provider {
	return &Provider{client: client, token: token}
}

// ConfigureWithOptions sets token and SDK options (tests against httptest).
func (p *Provider) ConfigureWithOptions(token, apiBase string, opts ...spritesdk.Option) error {
	if token == "" {
		return missingTokenErr()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client != nil {
		_ = p.client.Close()
	}
	p.token = token
	p.client = newSDKClient(token, apiBase, opts...)
	return nil
}
