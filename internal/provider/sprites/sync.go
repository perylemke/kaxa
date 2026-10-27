package sprites

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/kaxa/internal/sandbox"
)

const (
	workspaceDir = "/home/sprite/workspace"
	argvMkdir    = "mkdir"
	argvTar      = "tar"
	tarGzip      = "-xzf"
)

// HomeDir is the default working directory of a sprite.
func (p *Provider) HomeDir() string {
	return spriteHome
}

// RepoDir is where SyncWorkspace places the repository named repo.
func (p *Provider) RepoDir(repo string) (string, error) {
	if err := validateRepoDir(repo); err != nil {
		return "", err
	}
	return workspaceDir + "/" + repo, nil
}

// Available reports that Sprites can create sandboxes.
func (p *Provider) Available() bool {
	return true
}

func workspaceExecDir(labels []string) string {
	if slices.Contains(labels, sandbox.SyncLabel) {
		return workspaceDir
	}
	return ""
}

func spriteLabels(sp *spritesdk.Sprite) []string {
	if sp == nil {
		return nil
	}
	return sp.Labels
}

// RepoExists reports whether /home/sprite/workspace/<repo> exists. Exit status 1
// from test(1) means absent; any other failure is an error, not an answer.
func (p *Provider) RepoExists(ctx context.Context, id sandbox.ID, repo string) (bool, error) {
	if err := validateName(string(id)); err != nil {
		return false, err
	}
	dir, err := p.RepoDir(repo)
	if err != nil {
		return false, err
	}
	c, token, err := p.clientAndToken()
	if err != nil {
		return false, err
	}
	err = c.RunIn(ctx, string(id), "", nil, []string{"test", "-d", dir}, nil, io.Discard, io.Discard)
	if err == nil {
		return true, nil
	}
	mapped := mapExecErr(err, token)
	if exitErr, ok := errors.AsType[*sandbox.ExitError](mapped); ok {
		if exitErr.Code == 1 {
			return false, nil
		}
		// A plain error: the CLI turns an ExitError into a silent exit code.
		return false, fmt.Errorf("test -d %s exited with status %d", dir, exitErr.Code)
	}
	return false, mapped
}

// SyncWorkspace extracts a gzip tar from archive into /home/sprite/workspace.
func (p *Provider) SyncWorkspace(ctx context.Context, id sandbox.ID, archive io.Reader) error {
	if err := validateName(string(id)); err != nil {
		return err
	}
	if archive == nil {
		return errors.New("sprites sync: empty archive")
	}
	c, token, err := p.clientAndToken()
	if err != nil {
		return err
	}
	name := string(id)
	if _, err := runRemote(ctx, c, name, token, []string{argvMkdir, "-p", workspaceDir}); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	if _, err := runRemoteStdin(ctx, c, name, token, []string{argvTar, tarGzip, "-", "-C", workspaceDir}, archive); err != nil {
		return fmt.Errorf("extract workspace: %w", err)
	}
	return nil
}
