package cli_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/perylemke/sbox/internal/cli"
	"github.com/perylemke/sbox/internal/provider/sprites"
	"github.com/perylemke/sbox/internal/registry"
	"github.com/perylemke/sbox/internal/sandbox"
)

const (
	testProviderFlag = "--provider"
	testSpritesName  = "sprites"
)

type fakeProvider struct {
	name           string
	configureToken string
	createID       sandbox.ID
	list           []sandbox.Info
	lastExec       sandbox.ExecOptions
	destroyed      sandbox.ID
	execErr        error
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Configure(token string) error {
	f.configureToken = token
	if token == "" {
		return sprites.ErrMissingToken
	}
	return nil
}

func (f *fakeProvider) Create(_ context.Context, spec sandbox.Spec) (sandbox.ID, error) {
	if f.createID != "" {
		return f.createID, nil
	}
	return sandbox.ID(spec.Name), nil
}

func (f *fakeProvider) List(context.Context) ([]sandbox.Info, error) {
	return f.list, nil
}

func (f *fakeProvider) Exec(_ context.Context, _ sandbox.ID, opts sandbox.ExecOptions) error {
	f.lastExec = opts
	return f.execErr
}

func (f *fakeProvider) Destroy(_ context.Context, id sandbox.ID) error {
	f.destroyed = id
	return nil
}

func spritesRegistry(t *testing.T, p sandbox.Provider) *registry.Registry {
	t.Helper()
	reg := registry.New()
	if err := reg.Register(p); err != nil {
		t.Fatalf("register: %v", err)
	}
	return reg
}

func TestCreatePrintsName(t *testing.T) {
	fp := &fakeProvider{name: testSpritesName, createID: "my-box"}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, "create", "my-box"})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "my-box" {
		t.Fatalf("output = %q", got)
	}
	if fp.configureToken != "tok" {
		t.Fatalf("token = %q", fp.configureToken)
	}
}

func TestListFormatsOutput(t *testing.T) {
	fp := &fakeProvider{
		name: testSpritesName,
		list: []sandbox.Info{
			{ID: "a", Status: "cold"},
			{ID: "b", Status: "warm"},
		},
	}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, "list"})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	want := "a\tcold\nb\twarm\n"
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func TestExecWithArgsNoTTY(t *testing.T) {
	fp := &fakeProvider{name: testSpritesName}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, "exec", "box", "--", "ls", "-la"})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if fp.lastExec.TTY {
		t.Fatal("expected non-interactive exec")
	}
	if len(fp.lastExec.Command) != 2 || fp.lastExec.Command[0] != "ls" {
		t.Fatalf("command = %v", fp.lastExec.Command)
	}
}

func TestDestroyCommands(t *testing.T) {
	const sandboxName = "gone"

	for _, sub := range []string{"destroy", "rm"} {
		t.Run(sub, func(t *testing.T) {
			fp := &fakeProvider{name: testSpritesName}
			reg := spritesRegistry(t, fp)
			cmd := cli.NewRootCmd(reg)
			cmd.SetOut(new(bytes.Buffer))
			cmd.SetErr(new(bytes.Buffer))
			cmd.SetArgs([]string{testProviderFlag, testSpritesName, sub, sandboxName})
			t.Setenv("SBOX_SPRITES_TOKEN", "tok")

			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if fp.destroyed != sandboxName {
				t.Fatalf("destroyed = %q", fp.destroyed)
			}
		})
	}
}



func TestExecReturnsExitError(t *testing.T) {
	fp := &fakeProvider{
		name:    testSpritesName,
		execErr: &sandbox.ExitError{Code: 7},
	}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, "exec", "box", "false"})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	err := cmd.Execute()
	var exitErr *sandbox.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("want ExitError, got %v", err)
	}
	if exitErr.Code != 7 {
		t.Fatalf("code = %d", exitErr.Code)
	}
}

func TestSandboxSubcommandHelp(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testSpritesName}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"create", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "create") {
		t.Fatalf("help = %q", out.String())
	}
}
