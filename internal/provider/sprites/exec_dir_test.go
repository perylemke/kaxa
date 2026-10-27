package sprites

import (
	"context"
	"testing"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestPrepareExecCmdStartsInWorkspace(t *testing.T) {
	t.Parallel()

	cmd := execCmd(t, []string{"kaxa", sandbox.SyncLabel})
	if cmd.Dir != workspaceDir {
		t.Fatalf("dir = %q", cmd.Dir)
	}
}

func TestPrepareExecCmdKeepsDefaultDir(t *testing.T) {
	t.Parallel()

	cmd := execCmd(t, []string{"kaxa"})
	if cmd.Dir != "" {
		t.Fatalf("dir = %q", cmd.Dir)
	}
}

func execCmd(t *testing.T, labels []string) *spritesdk.Cmd {
	t.Helper()
	sprite := spritesdk.New("token").Sprite("box")
	cmd, err := prepareExecCmd(context.Background(), sprite, sandbox.ExecOptions{Command: []string{"bash"}}, labels)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	return cmd
}
