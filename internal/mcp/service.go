package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/perylemke/sbox/internal/config"
	"github.com/perylemke/sbox/internal/lifecycle"
	"github.com/perylemke/sbox/internal/registry"
	"github.com/perylemke/sbox/internal/sandbox"
	"github.com/perylemke/sbox/internal/usererr"
)

const defaultAPIPort = 8000

// service implements the sandbox tools on top of the provider registry.
type service struct {
	reg    *registry.Registry
	cfg    config.Config
	root   string
	stderr io.Writer
	log    *slog.Logger
}

func newService(reg *registry.Registry, cfg config.Config, opts Options) (*service, error) {
	root := opts.Root
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("working directory: %w", err)
		}
		root = wd
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	return &service{
		reg:    reg,
		cfg:    cfg,
		root:   root,
		stderr: stderr,
		log:    slog.New(slog.NewTextHandler(stderr, nil)),
	}, nil
}

// progress reports a slow step on stderr. Stdout carries only the protocol.
func (s *service) progress(ctx context.Context, label string, task func(context.Context) error) error {
	_, _ = fmt.Fprintln(s.stderr, label)
	return task(ctx)
}

func (s *service) provider(name string) (sandbox.Provider, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = s.cfg.Provider
	}
	if name != "" && !s.reg.Has(name) {
		return nil, usererr.New(
			fmt.Sprintf("unknown provider %q", name),
			"supported: "+strings.Join(s.reg.Names(), ", "),
		)
	}
	p, err := lifecycle.ResolveProvider(s.reg, s.cfg, name)
	if err != nil {
		return nil, err
	}
	if !sandbox.IsAvailable(p) {
		return nil, usererr.New(
			fmt.Sprintf("the %s provider is not available yet", p.Name()),
			"call list_providers and choose a provider that is available",
		)
	}
	return p, nil
}

func requireName(name string) (sandbox.ID, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", usererr.New("name is required", "pass the sandbox name")
	}
	return sandbox.ID(name), nil
}

func (s *service) listProviders(_ context.Context, _ listProvidersIn) (listProvidersOut, error) {
	names := s.reg.Names()
	out := listProvidersOut{Providers: make([]providerInfo, 0, len(names))}
	for _, name := range names {
		p, err := s.reg.Get(name)
		if err != nil {
			return listProvidersOut{}, err
		}
		out.Providers = append(out.Providers, providerInfo{
			Name:         name,
			Available:    sandbox.IsAvailable(p),
			Default:      name == s.cfg.Provider,
			Capabilities: sandbox.CapabilitiesOf(p),
		})
	}
	return out, nil
}

func (s *service) create(ctx context.Context, in createIn) (createOut, error) {
	p, err := s.provider(in.Provider)
	if err != nil {
		return createOut{}, err
	}
	name, err := requireName(in.Name)
	if err != nil {
		return createOut{}, err
	}
	req, err := s.createRequest(string(name), in)
	if err != nil {
		return createOut{}, err
	}
	res, err := lifecycle.Create(ctx, p, req, s.progress)
	if err != nil {
		if res.ID != "" {
			err = fmt.Errorf("%w (sandbox %s was kept)", err, res.ID)
		}
		return createOut{}, err
	}
	out := createOut{ID: string(res.ID), Provider: p.Name(), WorkDir: res.WorkDir}
	detail, err := p.Inspect(ctx, res.ID)
	if err != nil {
		s.log.Warn("inspect after create failed", "sandbox", res.ID, "error", err)
		return out, nil
	}
	out.URL, out.Auth = detail.URL, detail.Auth
	return out, nil
}

func (s *service) createRequest(name string, in createIn) (lifecycle.Request, error) {
	req := lifecycle.Request{Name: name, Running: in.Running, Sync: in.Sync, Public: in.Public}
	if err := applyAPI(&req, in); err != nil {
		return lifecycle.Request{}, err
	}
	if !in.Sync {
		if in.Dir != "" {
			return lifecycle.Request{}, usererr.New("dir requires sync", "set sync to copy that repository")
		}
		return req, nil
	}
	dir := in.Dir
	if dir == "" {
		dir = s.root
	}
	root, repo, err := lifecycle.ResolveRepo(dir, true)
	if err != nil {
		return lifecycle.Request{}, err
	}
	if err := checkRepoScope(root, s.root); err != nil {
		return lifecycle.Request{}, err
	}
	req.Root, req.Repo = root, repo
	return req, nil
}

func applyAPI(req *lifecycle.Request, in createIn) error {
	if !in.API {
		if len(in.APICmd) > 0 {
			return usererr.New("api_cmd requires api", "set api to publish the service")
		}
		if in.APIPort != 0 {
			return usererr.New("api_port requires api", "set api to publish the service")
		}
		return nil
	}
	port := in.APIPort
	if port == 0 {
		port = defaultAPIPort
	}
	if err := lifecycle.ValidateAPI(in.Running, port, in.APICmd); err != nil {
		return err
	}
	req.API, req.APIPort, req.APICmd = true, port, in.APICmd
	return nil
}

// checkRepoScope keeps an agent from copying an unrelated repository off the
// host: the repository must contain the server working directory or sit below it.
func checkRepoScope(repoRoot, scope string) error {
	repo, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		return fmt.Errorf("resolve repository: %w", err)
	}
	base, err := filepath.EvalSymlinks(scope)
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}
	if within(repo, base) || within(base, repo) {
		return nil
	}
	return usererr.New(
		fmt.Sprintf("repository %s is outside the server working directory", repoRoot),
		"start sbox mcp from the project, or pass a dir inside it",
	)
}

// within reports whether child is parent or lies below it.
func within(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func (s *service) exec(ctx context.Context, in execIn) (execOut, error) {
	p, err := s.provider(in.Provider)
	if err != nil {
		return execOut{}, err
	}
	id, err := requireName(in.Name)
	if err != nil {
		return execOut{}, err
	}
	if len(in.Command) == 0 || strings.TrimSpace(in.Command[0]) == "" {
		return execOut{}, usererr.New("command is required", "pass the argv to run; the first item is the binary")
	}
	stdout, stderr := newLimitedBuffer(maxOutputBytes), newLimitedBuffer(maxOutputBytes)
	err = p.Exec(ctx, id, sandbox.ExecOptions{Command: in.Command, Stdout: stdout, Stderr: stderr})
	out := execOut{
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		Truncated: stdout.wasTruncated() || stderr.wasTruncated(),
	}
	if exitErr, ok := errors.AsType[*sandbox.ExitError](err); ok {
		out.ExitCode = exitErr.Code
		return out, nil
	}
	if err != nil {
		return execOut{}, err
	}
	return out, nil
}

func (s *service) listSandboxes(ctx context.Context, in listIn) (listOut, error) {
	p, err := s.provider(in.Provider)
	if err != nil {
		return listOut{}, err
	}
	infos, err := p.List(ctx)
	if err != nil {
		return listOut{}, err
	}
	out := listOut{Sandboxes: make([]sandboxInfo, 0, len(infos))}
	for _, info := range infos {
		out.Sandboxes = append(out.Sandboxes, sandboxInfo{
			ID:       string(info.ID),
			Provider: info.Provider,
			Status:   info.Status,
			Labels:   nonNil(info.Labels),
		})
	}
	return out, nil
}

func (s *service) inspect(ctx context.Context, in nameIn) (inspectOut, error) {
	p, err := s.provider(in.Provider)
	if err != nil {
		return inspectOut{}, err
	}
	id, err := requireName(in.Name)
	if err != nil {
		return inspectOut{}, err
	}
	d, err := p.Inspect(ctx, id)
	if err != nil {
		return inspectOut{}, err
	}
	return inspectOut{
		ID:           string(d.ID),
		Provider:     d.Provider,
		Status:       d.Status,
		Labels:       nonNil(d.Labels),
		Organization: d.Organization,
		URL:          d.URL,
		Auth:         d.Auth,
		CreatedAt:    formatTime(d.CreatedAt),
		UpdatedAt:    formatTime(d.UpdatedAt),
	}, nil
}

func (s *service) destroy(ctx context.Context, in nameIn) (actionOut, error) {
	return s.act(ctx, in, "destroyed", sandbox.Provider.Destroy)
}

func (s *service) restart(ctx context.Context, in nameIn) (actionOut, error) {
	return s.act(ctx, in, "restarted", sandbox.Provider.Restart)
}

func (s *service) upgrade(ctx context.Context, in nameIn) (actionOut, error) {
	return s.act(ctx, in, "upgraded", sandbox.Provider.Upgrade)
}

func (s *service) act(ctx context.Context, in nameIn, status string, run func(sandbox.Provider, context.Context, sandbox.ID) error) (actionOut, error) {
	p, err := s.provider(in.Provider)
	if err != nil {
		return actionOut{}, err
	}
	id, err := requireName(in.Name)
	if err != nil {
		return actionOut{}, err
	}
	if err := run(p, ctx, id); err != nil {
		return actionOut{}, err
	}
	return actionOut{ID: string(id), Provider: p.Name(), Status: status}, nil
}

func (s *service) publishAPI(ctx context.Context, in publishIn) (actionOut, error) {
	p, err := s.provider(in.Provider)
	if err != nil {
		return actionOut{}, err
	}
	id, err := requireName(in.Name)
	if err != nil {
		return actionOut{}, err
	}
	publisher, ok := p.(sandbox.APIPublisher)
	if !ok {
		return actionOut{}, lifecycle.Unsupported(p, "api publish")
	}
	port := in.APIPort
	if port == 0 {
		port = defaultAPIPort
	}
	if err := lifecycle.ValidateAPI(false, port, in.APICmd); err != nil {
		return actionOut{}, err
	}
	workDir, err := publishDir(p, in.WorkDir)
	if err != nil {
		return actionOut{}, err
	}
	err = s.progress(ctx, "publishing api…", func(ctx context.Context) error {
		return publisher.PublishAPI(ctx, id, workDir, port, in.APICmd)
	})
	if err != nil {
		return actionOut{}, err
	}
	return actionOut{ID: string(id), Provider: p.Name(), Status: "published"}, nil
}

func publishDir(p sandbox.Provider, dir string) (string, error) {
	if dir == "" {
		layout, ok := p.(sandbox.WorkspaceLayout)
		if !ok {
			return "", usererr.New(
				fmt.Sprintf("the %s provider does not report a workspace layout", p.Name()),
				"pass work_dir",
			)
		}
		return layout.HomeDir(), nil
	}
	if strings.ContainsRune(dir, 0) || !path.IsAbs(dir) {
		return "", usererr.New("work_dir must be an absolute path", "pass a directory inside the sandbox")
	}
	return path.Clean(dir), nil
}

func (s *service) setURLAuth(ctx context.Context, in urlAuthIn) (urlAuthOut, error) {
	p, err := s.provider(in.Provider)
	if err != nil {
		return urlAuthOut{}, err
	}
	id, err := requireName(in.Name)
	if err != nil {
		return urlAuthOut{}, err
	}
	if in.Auth != sandbox.URLAuthPublic && in.Auth != sandbox.URLAuthSprite {
		return urlAuthOut{}, usererr.New(fmt.Sprintf("invalid url auth %q", in.Auth), "use public or sprite")
	}
	setter, ok := p.(sandbox.PublicURL)
	if !ok {
		return urlAuthOut{}, lifecycle.Unsupported(p, "url auth")
	}
	if err := setter.SetURLAuth(ctx, id, in.Auth); err != nil {
		return urlAuthOut{}, err
	}
	out := urlAuthOut{ID: string(id), Provider: p.Name(), Auth: in.Auth}
	if detail, err := p.Inspect(ctx, id); err == nil {
		out.URL = detail.URL
	} else {
		s.log.Warn("inspect after url auth failed", "sandbox", id, "error", err)
	}
	return out, nil
}

func nonNil(labels []string) []string {
	if labels == nil {
		return []string{}
	}
	return labels
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
