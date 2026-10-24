package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/perylemke/sbox/internal/cli"
	"github.com/perylemke/sbox/internal/provider/modal"
	"github.com/perylemke/sbox/internal/provider/sprites"
	"github.com/perylemke/sbox/internal/registry"
)

func testRegistry(t *testing.T) *registry.Registry {
	t.Helper()
	reg := registry.New()
	if err := reg.Register(sprites.New()); err != nil {
		t.Fatalf("register sprites: %v", err)
	}
	if err := reg.Register(modal.New()); err != nil {
		t.Fatalf("register modal: %v", err)
	}
	return reg
}

func TestRootHelp(t *testing.T) {
	t.Parallel()

	reg := testRegistry(t)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	help := out.String()
	if !strings.Contains(help, "sprites") || !strings.Contains(help, "modal") {
		t.Fatalf("help should list providers: %s", help)
	}
	for _, sub := range []string{testCmdCreate, testCmdList, "exec", "destroy"} {
		if !strings.Contains(help, sub) {
			t.Fatalf("help should list %q: %s", sub, help)
		}
	}
	for _, unwanted := range []string{"create, new", "list, ls", "destroy, rm", "(alias:"} {
		if strings.Contains(help, unwanted) {
			t.Fatalf("help should not include %q in command descriptions: %s", unwanted, help)
		}
	}
	for _, alias := range []string{"  new create", "  ls  list", "  rm  destroy"} {
		if !strings.Contains(help, alias) {
			t.Fatalf("help should list alias %q: %s", alias, help)
		}
	}
}

func TestVersionCommand(t *testing.T) {
	t.Parallel()

	reg := testRegistry(t)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "sbox dev") {
		t.Fatalf("version output = %q", got)
	}
	if !strings.Contains(got, "on ") {
		t.Fatalf("version should include platform line: %q", got)
	}
}

func TestVersionSkipsConfigValidation(t *testing.T) {
	reg := testRegistry(t)
	cmd := cli.NewRootCmd(reg)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{testProviderFlag, "nope", "version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("version should not validate provider: %v", err)
	}
}

func TestRootUnknownProvider(t *testing.T) {
	t.Parallel()

	reg := testRegistry(t)
	cmd := cli.NewRootCmd(reg)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{testProviderFlag, "nope"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestRootKnownProvider(t *testing.T) {
	t.Parallel()

	reg := testRegistry(t)
	cmd := cli.NewRootCmd(reg)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
}
