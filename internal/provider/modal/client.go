package modal

import (
	"context"
	"errors"
	"io"
	"slices"
	"sort"
	"time"

	sdk "github.com/modal-labs/modal-client/go"
)

const (
	appName        = "kaxa"
	imageTag       = "debian:bookworm-slim"
	nameTag        = "kaxa.name"
	labelTagValue  = "true"
	sandboxTimeout = 24 * time.Hour // Modal's maximum sandbox lifetime.
	idleTimeout    = time.Hour
)

// Record is one Modal sandbox as kaxa sees it.
type Record struct {
	ID     string
	Name   string
	Labels []string
	// ExitCode is nil while the sandbox runs.
	ExitCode *int
}

// ExecParams are the options of one command run in a sandbox.
type ExecParams struct {
	// PTY gives the command a terminal. Modal then merges stderr into stdout.
	PTY bool
	// Workdir is an absolute directory to start in. Empty means the image default.
	Workdir string
}

// Process is a command running inside a sandbox.
type Process interface {
	Stdin() io.WriteCloser
	Stdout() io.Reader
	Stderr() io.Reader
	// Wait blocks until the command exits and returns its exit code.
	Wait(ctx context.Context) (int, error)
	// Close releases the handle on the sandbox. The command is not stopped.
	Close()
}

type sdkClient interface {
	Create(ctx context.Context, name string, labels []string) (Record, error)
	List(ctx context.Context) ([]Record, error)
	Get(ctx context.Context, name string) (Record, error)
	Terminate(ctx context.Context, name string) error
	// Exec starts argv in the named sandbox.
	Exec(ctx context.Context, name string, argv []string, params ExecParams) (Process, error)
	Close()
}

type realSDKClient struct {
	c *sdk.Client
}

func newSDKClient(tokenID, tokenSecret string) (*realSDKClient, error) {
	c, err := sdk.NewClientWithOptions(&sdk.ClientParams{TokenID: tokenID, TokenSecret: tokenSecret})
	if err != nil {
		return nil, err
	}
	return &realSDKClient{c: c}, nil
}

func (r *realSDKClient) Close() { r.c.Close() }

func (r *realSDKClient) Create(ctx context.Context, name string, labels []string) (Record, error) {
	app, err := r.c.Apps.FromName(ctx, appName, &sdk.AppFromNameParams{CreateIfMissing: true})
	if err != nil {
		return Record{}, err
	}
	sb, err := r.c.Sandboxes.Create(ctx, app, r.c.Images.FromRegistry(imageTag, nil), &sdk.SandboxCreateParams{
		Name:        name,
		Tags:        tagsFor(name, labels),
		Command:     []string{"sleep", "infinity"},
		Timeout:     sandboxTimeout,
		IdleTimeout: idleTimeout,
	})
	if err != nil {
		if _, exists := errors.AsType[sdk.AlreadyExistsError](err); exists {
			return Record{}, ErrAlreadyExists
		}
		return Record{}, err
	}
	defer func() { _ = sb.Detach() }()
	return Record{ID: sb.SandboxID, Name: name, Labels: slices.Clone(labels)}, nil
}

func (r *realSDKClient) List(ctx context.Context) ([]Record, error) {
	app, err := r.c.Apps.FromName(ctx, appName, nil)
	if err != nil {
		if _, missing := errors.AsType[sdk.NotFoundError](err); missing {
			return nil, nil
		}
		return nil, err
	}
	seq, err := r.c.Sandboxes.List(ctx, &sdk.SandboxListParams{AppID: app.AppID})
	if err != nil {
		return nil, err
	}
	var out []Record
	for sb, err := range seq {
		if err != nil {
			return nil, err
		}
		rec, err := recordOf(ctx, sb)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

func (r *realSDKClient) Get(ctx context.Context, name string) (Record, error) {
	sb, err := r.c.Sandboxes.FromName(ctx, appName, name, nil)
	if err != nil {
		return Record{}, notFound(err)
	}
	return recordOf(ctx, sb)
}

func (r *realSDKClient) Terminate(ctx context.Context, name string) error {
	sb, err := r.c.Sandboxes.FromName(ctx, appName, name, nil)
	if err != nil {
		return notFound(err)
	}
	defer func() { _ = sb.Detach() }()
	// Wait so the sandbox is finished, not still stopping, when Destroy returns.
	_, err = sb.Terminate(ctx, &sdk.SandboxTerminateParams{Wait: true})
	return err
}

func (r *realSDKClient) Exec(ctx context.Context, name string, argv []string, params ExecParams) (Process, error) {
	sb, err := r.c.Sandboxes.FromName(ctx, appName, name, nil)
	if err != nil {
		return nil, notFound(err)
	}
	cp, err := sb.Exec(ctx, argv, &sdk.SandboxExecParams{PTY: params.PTY, Workdir: params.Workdir})
	if err != nil {
		_ = sb.Detach()
		return nil, err
	}
	return &sdkProcess{sb: sb, cp: cp}, nil
}

type sdkProcess struct {
	sb *sdk.Sandbox
	cp *sdk.ContainerProcess
}

func (p *sdkProcess) Stdin() io.WriteCloser { return p.cp.Stdin }
func (p *sdkProcess) Stdout() io.Reader     { return p.cp.Stdout }
func (p *sdkProcess) Stderr() io.Reader     { return p.cp.Stderr }
func (p *sdkProcess) Close()                { _ = p.sb.Detach() }

func (p *sdkProcess) Wait(ctx context.Context) (int, error) {
	return p.cp.Wait(ctx, nil)
}

// recordOf reads tags and status, then detaches the handle.
func recordOf(ctx context.Context, sb *sdk.Sandbox) (Record, error) {
	defer func() { _ = sb.Detach() }()
	tags, err := sb.GetTags(ctx, nil)
	if err != nil {
		return Record{}, err
	}
	code, err := sb.Poll(ctx, nil)
	if err != nil {
		return Record{}, err
	}
	name, labels := fromTags(tags)
	if name == "" {
		name = sb.SandboxID
	}
	return Record{ID: sb.SandboxID, Name: name, Labels: labels, ExitCode: code}, nil
}

func notFound(err error) error {
	if _, missing := errors.AsType[sdk.NotFoundError](err); missing {
		return ErrNotFound
	}
	return err
}

// tagsFor stores the name and each label as Modal tags. The SDK exposes no
// sandbox name on a handle, so the name tag is how List recovers it.
func tagsFor(name string, labels []string) map[string]string {
	tags := make(map[string]string, len(labels)+1)
	for _, l := range labels {
		if l != "" && l != nameTag {
			tags[l] = labelTagValue
		}
	}
	tags[nameTag] = name
	return tags
}

func fromTags(tags map[string]string) (string, []string) {
	var labels []string
	for k := range tags {
		if k != nameTag {
			labels = append(labels, k)
		}
	}
	sort.Strings(labels)
	return tags[nameTag], labels
}
