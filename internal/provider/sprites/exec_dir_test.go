package sprites

import (
	"context"
	"slices"
	"testing"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/kaxa/internal/sandbox"
)

const (
	spriteLabel = "kaxa"
	testShell   = "bash"
)

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
		Command: []string{testShell}, Dir: "/home/sprite/workspace/app/",
	}, []string{spriteLabel, sandbox.SyncLabel}, nil)
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
		Command: []string{testShell}, Dir: "app",
	}, []string{spriteLabel}, nil)
	if err == nil {
		t.Fatal("relative dir accepted")
	}
}

func TestPrepareExecCmdAppliesSpriteEnvironmentAfterToolchain(t *testing.T) {
	t.Parallel()

	sprite := spritesdk.New("token").Sprite("box")
	cmd, err := prepareExecCmd(context.Background(), sprite, sandbox.ExecOptions{
		Command: []string{testShell},
	}, []string{spriteLabel}, []string{"MODE=sandbox", "PATH=/evil"})
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !slices.Contains(cmd.Env, "MODE=sandbox") {
		t.Fatalf("env = %v", cmd.Env)
	}
	for _, entry := range cmd.Env {
		if entry == "PATH=/evil" {
			t.Fatalf("toolchain PATH was replaced: %v", cmd.Env)
		}
	}
}

func execCmd(t *testing.T, labels []string) *spritesdk.Cmd {
	t.Helper()
	sprite := spritesdk.New("token").Sprite("box")
	cmd, err := prepareExecCmd(context.Background(), sprite, sandbox.ExecOptions{Command: []string{testShell}}, labels, nil)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	return cmd
}

func TestPrepareExecCmdSetsTermOnlyWithATerminal(t *testing.T) {
	t.Parallel()

	sprite := spritesdk.New("token").Sprite("box")
	tty, err := prepareExecCmd(context.Background(), sprite, sandbox.ExecOptions{Command: []string{testShell}, TTY: true}, []string{spriteLabel}, nil)
	if err != nil || !slices.Contains(tty.Env, "TERM=xterm-256color") {
		t.Fatalf("tty env = %v, %v", tty.Env, err)
	}
	plain, err := prepareExecCmd(context.Background(), sprite, sandbox.ExecOptions{Command: []string{testShell}}, []string{spriteLabel}, nil)
	if err != nil || slices.Contains(plain.Env, "TERM=xterm-256color") {
		t.Fatalf("plain env = %v, %v", plain.Env, err)
	}
}
