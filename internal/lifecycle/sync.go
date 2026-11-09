package lifecycle

import (
	"context"
	"fmt"
	"slices"

	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

// SyncRequest describes repositories to copy into an existing sandbox.
type SyncRequest struct {
	// Name is the sandbox to copy into.
	Name string
	// Root and Repo locate one Git repository. Repos, when set, is the full list.
	// An empty Repos falls back to Root and Repo so existing callers keep working.
	Root  string
	Repo  string
	Repos []RepoCopy
	// NoInstall copies the repository and skips the toolchain and the dependency
	// commands, which the caller runs later.
	NoInstall bool
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
	copies := syncCopies(req)
	syncer, checker, err := syncSupport(p)
	if err != nil {
		return Result{}, err
	}
	id := sandbox.ID(req.Name)
	if err := checkSyncWorkspace(ctx, p, id, req.Name); err != nil {
		return Result{}, err
	}
	if err := checkReposAbsent(ctx, checker, id, req.Name, copies); err != nil {
		return Result{}, err
	}
	return copyAndInstallSynced(ctx, p, syncer, id, req, copies, run)
}

func syncCopies(req SyncRequest) []RepoCopy {
	if len(req.Repos) > 0 {
		return req.Repos
	}
	return []RepoCopy{{Root: req.Root, Repo: req.Repo}}
}

func copyAndInstallSynced(ctx context.Context, p sandbox.Provider, syncer sandbox.WorkspaceSyncer, id sandbox.ID, req SyncRequest, copies []RepoCopy, run Runner) (Result, error) {
	var skipped [][]string
	var workDir string
	for i, rc := range copies {
		steps, err := project.Detect(rc.Root)
		if err != nil {
			return Result{}, fmt.Errorf("install dependencies: %w", err)
		}
		repo := rc
		err = run(ctx, "syncing "+repo.Repo+"…", func(ctx context.Context) error {
			return copyWorkspace(ctx, syncer, id, repo.Root, repo.Repo)
		})
		if err != nil {
			return Result{}, fmt.Errorf("sync workspace: %w", err)
		}
		if err := installSynced(ctx, p, id, repo.Repo, steps, req.NoInstall, run); err != nil {
			return Result{}, err
		}
		skipped = append(skipped, skippedOf(steps, req.NoInstall)...)
		if i == 0 {
			dir, dirErr := syncedWorkDir(p, repo.Repo)
			if dirErr != nil {
				return Result{ID: id}, dirErr
			}
			workDir = dir
		}
	}
	return Result{ID: id, WorkDir: workDir, Skipped: skipped}, nil
}

func syncedWorkDir(p sandbox.Provider, repo string) (string, error) {
	layout, ok := p.(sandbox.WorkspaceLayout)
	if !ok {
		return "", nil
	}
	return layout.RepoDir(repo)
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

// checkSyncWorkspace refuses a sandbox that has no workspace.
func checkSyncWorkspace(ctx context.Context, p sandbox.Provider, id sandbox.ID, name string) error {
	detail, err := p.Inspect(ctx, id)
	if err != nil {
		return err
	}
	if _, limited := p.(sandbox.SyncRequiresCreate); limited && !slices.Contains(detail.Labels, sandbox.SyncLabel) {
		return usererr.New(
			fmt.Sprintf("sandbox %s was not created with sync", name),
			"only a sandbox created with sync has a workspace; create a new one",
		)
	}
	return nil
}

// checkReposAbsent refuses the whole sync when any repository is already in the sandbox.
func checkReposAbsent(ctx context.Context, checker sandbox.RepoChecker, id sandbox.ID, name string, copies []RepoCopy) error {
	for _, rc := range copies {
		exists, err := checker.RepoExists(ctx, id, rc.Repo)
		if err != nil {
			return fmt.Errorf("check repository: %w", err)
		}
		if exists {
			return usererr.New(
				fmt.Sprintf("%s already exists in sandbox %s", rc.Repo, name),
				"rename the local directory, or remove the old copy with exec",
			)
		}
	}
	return nil
}

func installSynced(ctx context.Context, p sandbox.Provider, id sandbox.ID, repo string, steps []project.Step, noInstall bool, run Runner) error {
	if len(steps) == 0 || noInstall {
		return nil
	}
	setup, ok := p.(sandbox.ProjectSetup)
	if !ok {
		return fmt.Errorf("install dependencies: %w", Unsupported(p, sandbox.FeatureDependencyInstall))
	}
	err := run(ctx, "installing "+repo+"…", func(ctx context.Context) error {
		return setup.SetupProjects(ctx, id, repo, steps)
	})
	if err != nil {
		return fmt.Errorf("install dependencies: %w", err)
	}
	return nil
}
