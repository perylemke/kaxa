package lifecycle

import (
	"context"
	"fmt"
	"slices"

	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

// SyncRequest describes one more repository to copy into an existing sandbox.
type SyncRequest struct {
	// Name is the sandbox to copy into.
	Name string
	// Root and Repo locate the Git repository to copy. See ResolveRepo.
	Root string
	Repo string
}

// Sync copies another Git repository into a sandbox that was created with
// --sync, then installs its dependencies. run reports progress; nil means none.
//
// It never replaces a directory: if the repository is already in the sandbox,
// Sync stops before copying. Unlike Create, a failure keeps the sandbox, which
// held other work before this call.
func Sync(ctx context.Context, p sandbox.Provider, req SyncRequest, run Runner) (Result, error) {
	if run == nil {
		run = Direct
	}
	syncer, checker, err := syncSupport(p)
	if err != nil {
		return Result{}, err
	}
	id := sandbox.ID(req.Name)
	if err := checkSyncTarget(ctx, p, checker, id, req); err != nil {
		return Result{}, err
	}
	err = run(ctx, "syncing "+req.Repo+"…", func(ctx context.Context) error {
		return copyWorkspace(ctx, syncer, id, req.Root, req.Repo)
	})
	if err != nil {
		return Result{}, fmt.Errorf("sync workspace: %w", err)
	}
	if err := installSynced(ctx, p, id, req, run); err != nil {
		return Result{}, err
	}
	res := Result{ID: id}
	if layout, ok := p.(sandbox.WorkspaceLayout); ok {
		if res.WorkDir, err = layout.RepoDir(req.Repo); err != nil {
			return res, err
		}
	}
	return res, nil
}

func syncSupport(p sandbox.Provider) (sandbox.WorkspaceSyncer, sandbox.RepoChecker, error) {
	syncer, ok := p.(sandbox.WorkspaceSyncer)
	if !ok {
		return nil, nil, Unsupported(p, sandbox.FeatureSync)
	}
	checker, ok := p.(sandbox.RepoChecker)
	if !ok {
		return nil, nil, Unsupported(p, sandbox.FeatureSync)
	}
	return syncer, checker, nil
}

// checkSyncTarget refuses a sandbox without a workspace and a repository that
// is already there.
func checkSyncTarget(ctx context.Context, p sandbox.Provider, checker sandbox.RepoChecker, id sandbox.ID, req SyncRequest) error {
	detail, err := p.Inspect(ctx, id)
	if err != nil {
		return err
	}
	if !slices.Contains(detail.Labels, sandbox.SyncLabel) {
		return usererr.New(
			fmt.Sprintf("sandbox %s was not created with sync", req.Name),
			"only a sandbox created with sync has a workspace; create a new one",
		)
	}
	exists, err := checker.RepoExists(ctx, id, req.Repo)
	if err != nil {
		return fmt.Errorf("check repository: %w", err)
	}
	if exists {
		return usererr.New(
			fmt.Sprintf("%s already exists in sandbox %s", req.Repo, req.Name),
			"rename the local directory, or remove the old copy with exec",
		)
	}
	return nil
}

func installSynced(ctx context.Context, p sandbox.Provider, id sandbox.ID, req SyncRequest, run Runner) error {
	steps, err := project.Detect(req.Root)
	if err != nil {
		return fmt.Errorf("install dependencies: %w", err)
	}
	if len(steps) == 0 {
		return nil
	}
	setup, ok := p.(sandbox.ProjectSetup)
	if !ok {
		return fmt.Errorf("install dependencies: %w", Unsupported(p, sandbox.FeatureDependencyInstall))
	}
	err = run(ctx, "installing "+req.Repo+"…", func(ctx context.Context) error {
		return setup.SetupProjects(ctx, id, req.Repo, steps)
	})
	if err != nil {
		return fmt.Errorf("install dependencies: %w", err)
	}
	return nil
}
