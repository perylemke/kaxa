package sprites

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/toolchain"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	spriteHome = "/home/sprite"
	spriteUser = "sprite"
)

// spriteLayout is where toolchains live on a sprite: under /home/sprite.
var spriteLayout = toolchain.Layout{Home: spriteHome, User: spriteUser}

// SetupProjects installs missing toolchains and runs each detected project command.
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
	c, token, err := p.clientAndToken()
	if err != nil {
		return err
	}
	r := spriteRunner{c: c, name: string(id), token: token}
	return toolchain.Setup(ctx, r, spriteLayout, workspaceDir+"/"+repoName, steps)
}

// spriteRunner runs toolchain commands on one sprite.
type spriteRunner struct {
	c     sdkClient
	name  string
	token string
}

func (r spriteRunner) Run(ctx context.Context, dir string, env, argv []string, stdin io.Reader) (string, error) {
	return runRemoteIn(ctx, r.c, r.name, r.token, dir, env, argv, stdin)
}

func validateRepoDir(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0) {
		return usererr.New(fmt.Sprintf("invalid repository directory %q", name), "use a single path segment")
	}
	return nil
}
