package sprites

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// envStub serves the sandbox env file and can fail the read.
type envStub struct {
	warmStubClient
	body string
	fail error
}

func (s *envStub) RunIn(_ context.Context, _, _ string, _, argv []string, _ io.Reader, stdout, _ io.Writer) error {
	if s.fail != nil {
		return s.fail
	}
	if len(argv) == 2 && argv[0] == "cat" {
		if s.body == "" {
			return &spritesdk.ExitError{Code: 1}
		}
		_, err := io.WriteString(stdout, s.body)
		return err
	}
	return nil
}

func TestExecReadsSandboxEnvFile(t *testing.T) {
	t.Parallel()

	client := &envStub{body: "MODE=sandbox\n"}
	entries, err := readSpriteEnv(context.Background(), client, "box", "tok")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	sprite := spritesdk.New("token").Sprite("box")
	cmd, err := prepareExecCmd(context.Background(), sprite, sandbox.ExecOptions{
		Command: []string{testShell},
	}, []string{spriteLabel}, entries)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !slices.Contains(cmd.Env, "MODE=sandbox") {
		t.Fatalf("env = %v", cmd.Env)
	}
	if cmd.Env[0] != "HOME=/home/sprite" {
		t.Fatalf("toolchain env is not first: %v", cmd.Env)
	}
}

func TestExecStopsWhenEnvReadFails(t *testing.T) {
	t.Parallel()

	p := NewWithClient(&envStub{fail: errors.New("read failed")}, "token")
	err := p.Exec(context.Background(), "box", sandbox.ExecOptions{Command: []string{testShell}})
	if err == nil || !strings.Contains(err.Error(), "read failed") {
		t.Fatalf("err = %v", err)
	}
}
