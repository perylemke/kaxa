package daytona

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/toolchain"
)

const (
	// daytonaUser is the unprivileged user of Daytona's default image. Every
	// command of kaxa runs as the default user of the sandbox, never as root.
	daytonaUser  = "daytona"
	daytonaHome  = "/home/daytona"
	workspaceDir = daytonaHome + "/workspace"

	archiveDir = "/tmp"
)

var daytonaLayout = toolchain.Layout{Home: daytonaHome, User: daytonaUser}

// HomeDir is the home directory of the user in a sandbox.
func (p *Provider) HomeDir() string {
	return daytonaHome
}

// RepoDir is where SyncWorkspace places the repository named repo.
func (p *Provider) RepoDir(repo string) (string, error) {
	if err := validateRepoDir(repo); err != nil {
		return "", err
	}
	return workspaceDir + "/" + repo, nil
}

// SyncWorkspace extracts a gzip tar from archive into /home/daytona/workspace.
// The archive is first uploaded to a temporary file, since the SDK takes a
// whole file and not a stream of input.
func (p *Provider) SyncWorkspace(ctx context.Context, id sandbox.ID, archive io.Reader) error {
	if archive == nil {
		return errors.New("daytona sync: empty archive")
	}
	s, err := p.open(ctx, id)
	if err != nil {
		return err
	}
	tmp, err := tempArchivePath()
	if err != nil {
		return err
	}
	// The temporary archive is removed even when the context is canceled.
	defer s.removeFile(tmp)
	if err := s.tb.Upload(ctx, s.rec.ID, tmp, archive); err != nil {
		return fmt.Errorf("upload workspace: %w", s.err(err))
	}
	if _, err := s.capture(ctx, "", nil, []string{"mkdir", "-p", workspaceDir}, nil); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	if _, err := s.capture(ctx, "", nil, []string{"tar", "-xzf", tmp, "-C", workspaceDir}, nil); err != nil {
		return fmt.Errorf("extract workspace: %w", err)
	}
	return nil
}

func tempArchivePath() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("create archive name: %w", err)
	}
	return archiveDir + "/kaxa-sync-" + hex.EncodeToString(b[:]) + ".tgz", nil
}

func (s *session) removeFile(path string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), cleanupTimeout)
	defer cancel()
	_, _ = s.capture(ctx, "", nil, []string{"rm", "-f", path}, nil)
}

// RepoExists reports whether /home/daytona/workspace/<repo> exists. Exit status
// 1 from test(1) means absent; any other failure is an error, not an answer.
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
	return toolchain.Setup(ctx, sessionRunner{s}, daytonaLayout, workspaceDir+"/"+repoName, steps)
}

// sessionRunner runs toolchain commands in one running sandbox.
type sessionRunner struct{ s *session }

func (r sessionRunner) Run(ctx context.Context, dir string, env, argv []string, stdin io.Reader) (string, error) {
	return r.s.capture(ctx, dir, env, argv, stdin)
}
