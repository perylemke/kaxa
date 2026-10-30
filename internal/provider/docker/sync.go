package docker

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
	// agentUser is the unprivileged user of the Docker agent images. Every
	// command of kaxa runs as that user, never as root.
	agentUser    = "agent"
	agentHome    = "/home/agent"
	workspaceDir = agentHome + "/workspace"
)

var cloudLayout = toolchain.Layout{Home: agentHome, User: agentUser}

// HomeDir is the home directory of the user in a sandbox.
func (p *Provider) HomeDir() string {
	return agentHome
}

// RepoDir is where SyncWorkspace places the repository named repo.
func (p *Provider) RepoDir(repo string) (string, error) {
	if err := validateRepoDir(repo); err != nil {
		return "", err
	}
	return workspaceDir + "/" + repo, nil
}

// SyncWorkspace extracts a gzip tar from archive into /home/agent/workspace.
// The archive is streamed to tar on standard input of a process in the
// sandbox; nothing is mounted from the host.
func (p *Provider) SyncWorkspace(ctx context.Context, id sandbox.ID, archive io.Reader) error {
	if archive == nil {
		return errors.New("docker sync: empty archive")
	}
	s, err := p.open(ctx, id)
	if err != nil {
		return err
	}
	if _, err := s.capture(ctx, "", nil, []string{"mkdir", "-p", workspaceDir}, nil); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	if _, err := s.capture(ctx, "", nil, []string{"tar", "-xzf", "-", "-C", workspaceDir}, archive); err != nil {
		return fmt.Errorf("extract workspace: %w", err)
	}
	return nil
}

// RepoExists reports whether /home/agent/workspace/<repo> exists. Exit status 1
// from test(1) means absent; any other failure is an error, not an answer.
func (p *Provider) RepoExists(ctx context.Context, id sandbox.ID, repo string) (bool, error) {
	dir, err := p.RepoDir(repo)
	if err != nil {
		return false, err
	}
	s, err := p.open(ctx, id)
	if err != nil {
		return false, err
	}
	err = s.run(ctx, runSpec{argv: []string{"test", "-d", dir}})
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
	return toolchain.Setup(ctx, sessionRunner{s}, cloudLayout, workspaceDir+"/"+repoName, steps)
}

// sessionRunner runs toolchain commands in one sandbox.
type sessionRunner struct{ s *session }

func (r sessionRunner) Run(ctx context.Context, dir string, env, argv []string, stdin io.Reader) (string, error) {
	return r.s.capture(ctx, dir, append(cloudLayout.Env(), env...), argv, stdin)
}
