package sprites

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/sbox/internal/sandbox"
)

const workspaceDir = "/home/sprite/workspace"

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
