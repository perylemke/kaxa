// Package lifecycle creates sandboxes for every adapter (CLI and MCP).
//
// It owns the create flow: capability checks, copy of the Git repository,
// dependency install, API publish, public URL and the rollback that deletes
// a sandbox when a later step fails. Adapters own flags, prompts and output.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/reposync"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	destroyTimeout = 15 * time.Second
	kaxaLabel      = "kaxa"
)

// Request describes one sandbox to create.
type Request struct {
	Name    string
	Running bool
	Sync    bool
	API     bool
	APIPort int
	APICmd  []string
	// APIEnv holds resolved KEY=VALUE entries for the API service. See ResolveAPIEnv.
	APIEnv []string
	Public bool
	// Template is the template to start from. Empty means the provider default.
	Template string
	// Root and Repo locate the Git repository to copy. Both are set when Sync is true.
	Root string
	Repo string
}

// Labels returns the provider labels for the sandbox.
func (r Request) Labels() []string {
	labels := []string{kaxaLabel}
	if r.Sync {
		labels = append(labels, sandbox.SyncLabel)
	}
	return labels
}

// Result describes a created sandbox.
type Result struct {
	ID sandbox.ID
	// WorkDir is the synced repository directory inside the sandbox. It is empty without Sync.
	WorkDir string
}

// Runner runs one slow step. Adapters use it to report progress, for example
// with a spinner or a line on stderr. The task must run to completion.
type Runner func(ctx context.Context, label string, task func(context.Context) error) error

// Direct runs the task without reporting progress.
func Direct(ctx context.Context, _ string, task func(context.Context) error) error {
	return task(ctx)
}

// Check rejects a request the provider cannot serve, before anything is created.
func Check(p sandbox.Provider, req Request) error {
	if req.Running {
		if _, ok := p.(sandbox.RunningSupport); !ok {
			return Unsupported(p, sandbox.FeatureRunning)
		}
	}
	if req.Template != "" {
		if _, ok := p.(sandbox.TemplateSupport); !ok {
			return Unsupported(p, sandbox.FeatureTemplate)
		}
	}
	if req.Sync {
		if _, ok := p.(sandbox.WorkspaceSyncer); !ok {
			return Unsupported(p, sandbox.FeatureSync)
		}
	}
	if req.API {
		if err := checkAPI(p, req); err != nil {
			return err
		}
	}
	if req.Public {
		if _, ok := p.(sandbox.PublicURL); !ok {
			return Unsupported(p, sandbox.FeaturePublicURLs)
		}
		if req.Running && !req.API {
			return sandbox.PublicTerminalError()
		}
	}
	return nil
}

func checkAPI(p sandbox.Provider, req Request) error {
	if _, ok := p.(sandbox.APIPublisher); !ok {
		return Unsupported(p, sandbox.FeatureAPIPublish)
	}
	if _, err := apiWorkDir(p, req); err != nil {
		return err
	}
	if !req.Sync {
		return nil
	}
	ok, err := project.IsAPI(req.Root)
	if err != nil {
		return err
	}
	if !ok {
		return usererr.New(
			"this repository is not an HTTP API",
			"omit the api option, or create from an API project",
		)
	}
	return nil
}

// Create runs the create flow. run reports progress; nil means no progress.
//
// A failed copy, install or API publish deletes the sandbox and returns a zero
// Result. A failed public URL change keeps the sandbox: the Result carries its
// ID next to the error.
func Create(ctx context.Context, p sandbox.Provider, req Request, run Runner) (Result, error) {
	if run == nil {
		run = Direct
	}
	if err := Check(p, req); err != nil {
		return Result{}, err
	}
	id, err := createSandbox(ctx, p, req, run)
	if err != nil {
		return Result{}, err
	}
	if err := syncAndInstall(ctx, p, id, req, run); err != nil {
		return Result{}, err
	}
	if err := publishAPI(ctx, p, id, req, run); err != nil {
		return Result{}, err
	}
	res := Result{ID: id}
	if req.Sync {
		if layout, ok := p.(sandbox.WorkspaceLayout); ok {
			if res.WorkDir, err = layout.RepoDir(req.Repo); err != nil {
				return res, err
			}
		}
	}
	return res, makePublic(ctx, p, id, req)
}

func createSandbox(ctx context.Context, p sandbox.Provider, req Request, run Runner) (sandbox.ID, error) {
	var id sandbox.ID
	err := run(ctx, "creating "+req.Name+" on "+p.Name()+"…", func(ctx context.Context) error {
		var createErr error
		id, createErr = p.Create(ctx, sandbox.Spec{
			Name:      req.Name,
			Labels:    req.Labels(),
			Running:   req.Running,
			ExposeAPI: req.API,
			APIPort:   apiPortOf(req),
			Template:  req.Template,
		})
		return createErr
	})
	return id, err
}

// apiPortOf is the port a published API will use, or 0 when there is no API.
func apiPortOf(req Request) int {
	if !req.API {
		return 0
	}
	return req.APIPort
}

func syncAndInstall(ctx context.Context, p sandbox.Provider, id sandbox.ID, req Request, run Runner) error {
	if !req.Sync {
		return nil
	}
	syncer, ok := p.(sandbox.WorkspaceSyncer)
	if !ok {
		return destroyAfterFailure(ctx, p, id, Unsupported(p, sandbox.FeatureSync), "sync workspace")
	}
	err := run(ctx, "syncing "+req.Repo+"…", func(ctx context.Context) error {
		return copyWorkspace(ctx, syncer, id, req.Root, req.Repo)
	})
	if err != nil {
		return destroyAfterFailure(ctx, p, id, err, "sync workspace")
	}
	return installProjects(ctx, p, id, req, run)
}

func installProjects(ctx context.Context, p sandbox.Provider, id sandbox.ID, req Request, run Runner) error {
	steps, err := project.Detect(req.Root)
	if err != nil {
		return destroyAfterFailure(ctx, p, id, err, "install dependencies")
	}
	if len(steps) == 0 {
		return nil
	}
	setup, ok := p.(sandbox.ProjectSetup)
	if !ok {
		return destroyAfterFailure(ctx, p, id, Unsupported(p, sandbox.FeatureDependencyInstall), "install dependencies")
	}
	err = run(ctx, "installing "+req.Repo+"…", func(ctx context.Context) error {
		return setup.SetupProjects(ctx, id, req.Repo, steps)
	})
	if err != nil {
		return destroyAfterFailure(ctx, p, id, err, "install dependencies")
	}
	return nil
}

func copyWorkspace(ctx context.Context, syncer sandbox.WorkspaceSyncer, id sandbox.ID, root, repoName string) error {
	archive, err := reposync.Archive(ctx, root, repoName)
	if err != nil {
		return err
	}
	syncErr := syncer.SyncWorkspace(ctx, id, archive)
	closeErr := archive.Close()
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return fmt.Errorf("close archive: %w", closeErr)
	}
	return nil
}

func publishAPI(ctx context.Context, p sandbox.Provider, id sandbox.ID, req Request, run Runner) error {
	if !req.API {
		return nil
	}
	publisher, ok := p.(sandbox.APIPublisher)
	if !ok {
		return destroyAfterFailure(ctx, p, id, Unsupported(p, sandbox.FeatureAPIPublish), "publish api")
	}
	workDir, err := apiWorkDir(p, req)
	if err != nil {
		return destroyAfterFailure(ctx, p, id, err, "publish api")
	}
	err = run(ctx, "publishing api…", func(ctx context.Context) error {
		return publisher.PublishAPI(ctx, id, workDir, req.APIPort, req.APICmd, req.APIEnv)
	})
	if err != nil {
		return destroyAfterFailure(ctx, p, id, err, "publish api")
	}
	return nil
}

func makePublic(ctx context.Context, p sandbox.Provider, id sandbox.ID, req Request) error {
	if !req.Public {
		return nil
	}
	setter, ok := p.(sandbox.PublicURL)
	if !ok {
		return Unsupported(p, sandbox.FeaturePublicURLs)
	}
	if err := setter.SetURLAuth(ctx, id, sandbox.URLAuthPublic); err != nil {
		return fmt.Errorf("set public url: %w", err)
	}
	return nil
}

func apiWorkDir(p sandbox.Provider, req Request) (string, error) {
	layout, ok := p.(sandbox.WorkspaceLayout)
	if !ok {
		return "", usererr.New(
			fmt.Sprintf("the %s provider does not report a workspace layout", p.Name()),
			"choose a provider that supports api publish",
		)
	}
	if !req.Sync {
		return layout.HomeDir(), nil
	}
	return layout.RepoDir(req.Repo)
}

func destroyAfterFailure(ctx context.Context, p sandbox.Provider, id sandbox.ID, cause error, action string) error {
	dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), destroyTimeout)
	defer cancel()
	wrapped := fmt.Errorf("%s: %w", action, cause)
	if err := p.Destroy(dctx, id); err != nil {
		return errors.Join(wrapped, fmt.Errorf("destroy sandbox: %w", err))
	}
	return wrapped
}

// Unsupported reports that p does not implement feature. The error wraps a
// sandbox.UnsupportedError; adapters call WithAlternatives to name the
// providers that do implement it.
func Unsupported(p sandbox.Provider, feature sandbox.Feature) error {
	cause := &sandbox.UnsupportedError{Provider: p.Name(), Feature: feature}
	return usererr.Wrap(cause, cause.Error(), "choose a provider that supports it")
}
