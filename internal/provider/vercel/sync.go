package vercel

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/toolchain"
)

const (
	// vercelUser is the user commands run as in a Vercel sandbox.
	vercelUser = "vercel-sandbox"
	// vercelHome is the default working directory of a sandbox.
	vercelHome   = "/vercel/sandbox"
	workspaceDir = vercelHome + "/workspace"
)

var vercelLayout = toolchain.Layout{Home: vercelHome, User: vercelUser}

// HomeDir is the default working directory of a sandbox.
func (p *Provider) HomeDir() string { return vercelHome }

// RepoDir is where SyncWorkspace places the repository named repo.
func (p *Provider) RepoDir(repo string) (string, error) {
	if err := validateRepoDir(repo); err != nil {
		return "", err
	}
	return workspaceDir + "/" + repo, nil
}

// PrepareWorkspace creates the workspace directory.
func (p *Provider) PrepareWorkspace(ctx context.Context, id sandbox.ID) error {
	s, err := p.open(ctx, id)
	if err != nil {
		return err
	}
	if err := s.api.Mkdir(ctx, s.rec.SessionID, workspaceDir); err != nil {
		return fmt.Errorf("create workspace: %w", s.err(err))
	}
	return nil
}

// SyncWorkspace extracts a gzip tar from archive into the workspace directory.
func (p *Provider) SyncWorkspace(ctx context.Context, id sandbox.ID, archive io.Reader) error {
	if archive == nil {
		return errors.New("vercel sync: empty archive")
	}
	s, err := p.open(ctx, id)
	if err != nil {
		return err
	}
	if err := s.api.Mkdir(ctx, s.rec.SessionID, workspaceDir); err != nil {
		return fmt.Errorf("create workspace: %w", s.err(err))
	}
	if err := s.api.WriteTar(ctx, s.rec.SessionID, workspaceDir, archive); err != nil {
		return fmt.Errorf("upload workspace: %w", s.err(err))
	}
	return nil
}

// RepoExists reports whether the workspace directory of repo exists. Exit
// status 1 from test(1) means absent; any other failure is an error.
func (p *Provider) RepoExists(ctx context.Context, id sandbox.ID, repo string) (bool, error) {
	dir, err := p.RepoDir(repo)
	if err != nil {
		return false, err
	}
	s, err := p.open(ctx, id)
	if err != nil {
		return false, err
	}
	err = s.run(ctx, runSpec{argv: []string{"test", "-d", dir}, dir: vercelHome})
	if err == nil {
		return true, nil
	}
	if exitErr, ok := errors.AsType[*sandbox.ExitError](err); ok {
		if exitErr.Code == 1 {
			return false, nil
		}
		// A plain error: the CLI turns an ExitError into a silent exit code.
		return false, fmt.Errorf("test -d %s exited with status %d", dir, exitErr.Code)
	}
	return false, err
}

// SetupProjects installs missing toolchains and runs each detected project
// command in the synced repository.
func (p *Provider) SetupProjects(ctx context.Context, id sandbox.ID, repoName string, steps []project.Step) error {
	if err := validateRepoDir(repoName); err != nil {
		return err
	}
	if len(steps) == 0 {
		return nil
	}
	s, err := p.open(ctx, id)
	if err != nil {
		return err
	}
	return toolchain.Setup(ctx, sessionRunner{s}, vercelLayout, workspaceDir+"/"+repoName, steps)
}

// sessionRunner runs toolchain commands in one resumed sandbox.
type sessionRunner struct{ s *session }

func (r sessionRunner) Run(ctx context.Context, dir string, env, argv []string, stdin io.Reader) (string, error) {
	return r.s.capture(ctx, dir, env, argv, stdin)
}
