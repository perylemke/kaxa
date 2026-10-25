package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/perylemke/sbox/internal/project"
	"github.com/perylemke/sbox/internal/reposync"
	"github.com/perylemke/sbox/internal/sandbox"
	"github.com/perylemke/sbox/internal/usererr"
)

const syncDestroyTimeout = 15 * time.Second

type createRequest struct {
	Name    string
	Running bool
	Sync    bool
	API     bool
	APIPort int
	APICmd  []string
	Public  bool
	Root    string
	Repo    string
}

func withRepo(req createRequest, flags createFlags) (createRequest, error) {
	if !req.Sync {
		if flags.DirSet {
			return createRequest{}, usererr.New(
				"--dir requires --sync",
				"pass --sync to copy that repository",
			)
		}
		return req, nil
	}
	start, explicit, err := syncStart(flags)
	if err != nil {
		return createRequest{}, err
	}
	root, name, err := reposync.FindRoot(start)
	if err != nil {
		return createRequest{}, syncRepoError(err, explicit)
	}
	req.Root = root
	req.Repo = name
	return req, nil
}

func syncStart(flags createFlags) (string, string, error) {
	if !flags.DirSet {
		wd, err := os.Getwd()
		if err != nil {
			return "", "", fmt.Errorf("working directory: %w", err)
		}
		return wd, "", nil
	}
	dir := strings.TrimSpace(flags.Dir)
	if dir == "" {
		return "", "", usererr.New("directory is required", "pass a path to a git repository")
	}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", usererr.New(
				fmt.Sprintf("directory %q was not found", dir),
				"pass an existing directory",
			)
		}
		return "", "", fmt.Errorf("stat directory: %w", err)
	}
	if !info.IsDir() {
		return "", "", usererr.New(
			fmt.Sprintf("%q is not a directory", dir),
			"pass a directory inside a git repository",
		)
	}
	return dir, dir, nil
}

func syncRepoError(err error, dir string) error {
	switch {
	case errors.Is(err, reposync.ErrNotRepository):
		if dir != "" {
			return usererr.Wrap(
				err,
				dir+" is not inside a git repository",
				"pass a directory inside a git repository",
			)
		}
		return usererr.Wrap(err, "current directory is not inside a git repository", "run sbox create --sync from a git repository")
	case errors.Is(err, reposync.ErrGitDirFile):
		return usererr.Wrap(err, ".git is not a directory", "sync does not follow linked worktrees")
	case errors.Is(err, reposync.ErrInvalidName):
		return usererr.Wrap(err, "invalid repository directory name", "use a directory name that is a single path segment")
	default:
		return err
	}
}

func workspaceSyncer(p sandbox.Provider, req createRequest) (sandbox.WorkspaceSyncer, error) {
	if !req.Sync {
		return nil, nil
	}
	syncer, ok := p.(sandbox.WorkspaceSyncer)
	if !ok {
		return nil, usererr.New(
			"sync is only supported for the sprites provider",
			"omit --sync or pass --provider sprites",
		)
	}
	return syncer, nil
}

func syncIfRequested(ctx context.Context, errOut io.Writer, p sandbox.Provider, syncer sandbox.WorkspaceSyncer, id sandbox.ID, req createRequest) error {
	if syncer == nil {
		return nil
	}
	err := runTaskWithSpinner(ctx, errOut, "syncing "+req.Repo+"…", func(ctx context.Context) error {
		return copyWorkspace(ctx, syncer, id, req.Root, req.Repo)
	})
	if err != nil {
		return destroyAfterFailure(ctx, p, id, err, "sync workspace")
	}
	return installProjects(ctx, errOut, p, id, req)
}

func installProjects(ctx context.Context, errOut io.Writer, p sandbox.Provider, id sandbox.ID, req createRequest) error {
	steps, err := project.Detect(req.Root)
	if err != nil {
		return destroyAfterFailure(ctx, p, id, err, "install dependencies")
	}
	if len(steps) == 0 {
		return nil
	}
	setup, ok := p.(sandbox.ProjectSetup)
	if !ok {
		return destroyAfterFailure(ctx, p, id, usererr.New(
			"dependency install is only supported for the sprites provider",
			"omit --sync or pass --provider sprites",
		), "install dependencies")
	}
	err = runTaskWithSpinner(ctx, errOut, "installing "+req.Repo+"…", func(ctx context.Context) error {
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

func destroyAfterFailure(ctx context.Context, p sandbox.Provider, id sandbox.ID, cause error, action string) error {
	dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), syncDestroyTimeout)
	defer cancel()
	wrapped := fmt.Errorf("%s: %w", action, cause)
	if err := p.Destroy(dctx, id); err != nil {
		return errors.Join(wrapped, fmt.Errorf("destroy sandbox: %w", err))
	}
	return wrapped
}
