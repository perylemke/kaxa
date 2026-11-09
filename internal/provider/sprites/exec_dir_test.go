package sprites

import (
	"context"
	"testing"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/kaxa/internal/sandbox"
)

const spriteLabel = "kaxa"

func TestPrepareExecCmdStartsInWorkspace(t *testing.T) {
	t.Parallel()

	cmd := execCmd(t, []string{spriteLabel, sandbox.SyncLabel})
	if cmd.Dir != workspaceDir {
		t.Fatalf("dir = %q", cmd.Dir)
	}
}

func TestPrepareExecCmdKeepsDefaultDir(t *testing.T) {
	t.Parallel()

	cmd := execCmd(t, []string{spriteLabel})
	if cmd.Dir != "" {
		t.Fatalf("dir = %q", cmd.Dir)
	}
}

func TestPrepareExecCmdUsesRequestedDir(t *testing.T) {
	t.Parallel()

	sprite := spritesdk.New("token").Sprite("box")
	cmd, err := prepareExecCmd(context.Background(), sprite, sandbox.ExecOptions{
		Command: []string{"bash"}, Dir: "/home/sprite/workspace/app/",
	}, []string{spriteLabel, sandbox.SyncLabel})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if cmd.Dir != "/home/sprite/workspace/app" {
		t.Fatalf("dir = %q", cmd.Dir)
	}
}

func TestPrepareExecCmdRejectsRelativeDir(t *testing.T) {
	t.Parallel()

	sprite := spritesdk.New("token").Sprite("box")
	_, err := prepareExecCmd(context.Background(), sprite, sandbox.ExecOptions{
		Command: []string{"bash"}, Dir: "app",
	}, []string{spriteLabel})
	if err == nil {
		t.Fatal("relative dir accepted")
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
