package modal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/toolchain"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	// modalUser owns the workspace. Commands of a synced sandbox run as this user,
	// never as root.
	modalUser    = "modal"
	modalHome    = "/home/modal"
	workspaceDir = modalHome + "/workspace"
)

var modalLayout = toolchain.Layout{Home: modalHome, User: modalUser}

// HomeDir is the home directory of the modal user in a synced sandbox.
func (p *Provider) HomeDir() string {
	return modalHome
}

// RepoDir is where SyncWorkspace places the repository named repo.
func (p *Provider) RepoDir(repo string) (string, error) {
	if err := validateRepoDir(repo); err != nil {
		return "", err
	}
	return workspaceDir + "/" + repo, nil
}

// SyncWorkspace extracts a gzip tar from archive into /home/modal/workspace.
// The sandbox must have been created with the sync label, which gives it the
// modal user and the workspace directory.
func (p *Provider) SyncWorkspace(ctx context.Context, id sandbox.ID, archive io.Reader) error {
	if err := validateName(string(id)); err != nil {
		return err
	}
	if archive == nil {
		return errors.New("modal sync: empty archive")
	}
	_, err := p.runAsModal(ctx, id, "", nil, []string{"tar", "-xzf", "-", "-C", workspaceDir}, archive)
	if err != nil {
		return fmt.Errorf("extract workspace: %w", err)
	}
	return nil
}

// SetupProjects installs missing toolchains as the modal user and runs each
// detected project command in the synced repository.
func (p *Provider) SetupProjects(ctx context.Context, id sandbox.ID, repoName string, steps []project.Step) error {
	if err := validateName(string(id)); err != nil {
		return err
	}
	if err := validateRepoDir(repoName); err != nil {
		return err
	}
	if len(steps) == 0 {
		return nil
	}
	r := modalRunner{p: p, id: id}
	return toolchain.Setup(ctx, r, modalLayout, workspaceDir+"/"+repoName, steps)
}

func validateRepoDir(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0) {
		return usererr.New(fmt.Sprintf("invalid repository directory %q", name), "use a single path segment")
	}
	return nil
}

// modalRunner runs toolchain commands as the modal user in one sandbox.
type modalRunner struct {
	p  *Provider
	id sandbox.ID
}

func (r modalRunner) Run(ctx context.Context, dir string, env, argv []string, stdin io.Reader) (string, error) {
	return r.p.runAsModal(ctx, r.id, dir, env, argv, stdin)
}

// asModal runs argv as the modal user. The environment goes through env(1)
// because runuser may reset it; later entries win, so env overrides the defaults.
// Nothing is parsed by a shell.
func asModal(env, argv []string) []string {
	out := []string{
		"runuser", "-u", modalUser, "--",
		"env", "HOME=" + modalHome, "USER=" + modalUser, "LOGNAME=" + modalUser,
	}
	out = append(out, env...)
	return append(out, argv...)
}

func isSynced(rec Record) bool {
	return slices.Contains(rec.Labels, sandbox.SyncLabel)
}

// runAsModal runs argv as the modal user and returns stdout. A failure carries
// the command's stderr, which the installer needs to explain what went wrong.
//
// A non-zero exit is returned as a plain error, not *sandbox.ExitError: the CLI
// turns an ExitError into the exit code of the process with no message, which
// is right for kaxa exec but would hide why a copy or install failed.
func (p *Provider) runAsModal(ctx context.Context, id sandbox.ID, dir string, env, argv []string, stdin io.Reader) (string, error) {
	var stdout, stderr bytes.Buffer
	opts := sandbox.ExecOptions{Command: asModal(env, argv), Stdin: stdin, Stdout: &stdout, Stderr: &stderr}
	err := p.run(ctx, id, dir, opts)
	if err == nil {
		return stdout.String(), nil
	}
	detail := strings.TrimSpace(stderr.String())
	if exitErr, ok := errors.AsType[*sandbox.ExitError](err); ok {
		msg := exitErr.Error()
		if detail != "" {
			msg += ": " + detail
		}
		return "", errors.New(msg)
	}
	if detail != "" {
		return "", fmt.Errorf("%w: %s", err, detail)
	}
	return "", err
}
