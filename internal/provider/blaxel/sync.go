package blaxel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/toolchain"
)

const (
	// blaxelUser is the user of blaxel/base-image. Blaxel runs a sandbox as root
	// unless the image declares another user.
	blaxelUser   = "root"
	blaxelHome   = "/root"
	workspaceDir = blaxelHome + "/workspace"
	archiveDir   = "/tmp"
)

var blaxelLayout = toolchain.Layout{Home: blaxelHome, User: blaxelUser}

// HomeDir is the home directory of the default image.
func (p *Provider) HomeDir() string { return blaxelHome }

// RepoDir is where SyncWorkspace places the repository named repo.
func (p *Provider) RepoDir(repo string) (string, error) {
	if err := validateRepoDir(repo); err != nil {
		return "", err
	}
	return workspaceDir + "/" + repo, nil
}

// PrepareWorkspace creates /root/workspace.
func (p *Provider) PrepareWorkspace(ctx context.Context, id sandbox.ID) error {
	_, err := p.capture(ctx, id, "", nil, []string{"mkdir", "-p", workspaceDir}, nil)
	return err
}

// SyncWorkspace extracts a gzip tar from archive into /root/workspace.
func (p *Provider) SyncWorkspace(ctx context.Context, id sandbox.ID, archive io.Reader) error {
	if archive == nil {
		return errors.New("blaxel sync: empty archive")
	}
	data, err := io.ReadAll(archive)
	if err != nil {
		return fmt.Errorf("read workspace archive: %w", err)
	}
	rec, secrets, err := p.open(ctx, id)
	if err != nil {
		return err
	}
	_, rt, _, err := p.clients()
	if err != nil {
		return err
	}
	tmp, err := tempArchivePath()
	if err != nil {
		return err
	}
	defer p.removeFile(id, tmp)
	if err := rt.Upload(ctx, rec.Name, tmp, data); err != nil {
		return fmt.Errorf("upload workspace: %w", userErr(err, rec.Name, secrets...))
	}
	if _, err := p.capture(ctx, id, "", nil, []string{"mkdir", "-p", workspaceDir}, nil); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	if _, err := p.capture(ctx, id, "", nil, []string{"tar", "-xzf", tmp, "-C", workspaceDir}, nil); err != nil {
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

func (p *Provider) removeFile(id sandbox.ID, path string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), cleanupTimeout)
	defer cancel()
	_, _ = p.capture(ctx, id, "", nil, []string{"rm", "-f", path}, nil)
}

// RepoExists reports whether /root/workspace/<repo> exists. Exit status 1 from
// test(1) means absent.
func (p *Provider) RepoExists(ctx context.Context, id sandbox.ID, repo string) (bool, error) {
	dir, err := p.RepoDir(repo)
	if err != nil {
		return false, err
	}
	_, err = p.capture(ctx, id, "", nil, []string{"test", "-d", dir}, nil)
	if err == nil {
		return true, nil
	}
	if code, ok := exitStatus(err); ok && code == 1 {
		return false, nil
	}
	return false, err
}

// exitStatus reads the code capture puts in a failed command error.
func exitStatus(err error) (int, bool) {
	rest, ok := strings.CutPrefix(err.Error(), "exit status ")
	if !ok {
		return 0, false
	}
	rest, _, _ = strings.Cut(rest, ":")
	code, convErr := strconv.Atoi(strings.TrimSpace(rest))
	if convErr != nil {
		return 0, false
	}
	return code, true
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
	if _, _, err := p.open(ctx, id); err != nil {
		return err
	}
	return toolchain.Setup(ctx, captureRunner{p: p, id: id}, blaxelLayout, workspaceDir+"/"+repoName, steps)
}

type captureRunner struct {
	p  *Provider
	id sandbox.ID
}

func (r captureRunner) Run(ctx context.Context, dir string, env, argv []string, stdin io.Reader) (string, error) {
	return r.p.capture(ctx, r.id, dir, env, argv, stdin)
}
