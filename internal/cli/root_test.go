package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/perylemke/sbx/internal/cli"
	"github.com/perylemke/sbx/internal/provider/modal"
	"github.com/perylemke/sbx/internal/provider/sprites"
	"github.com/perylemke/sbx/internal/registry"
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
	if !strings.Contains(got, "sbx dev") {
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
	cmd.SetArgs([]string{"--provider", "nope", "version"})

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
	cmd.SetArgs([]string{"--provider", "nope"})

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
	cmd.SetArgs([]string{"--provider", "sprites", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
}
