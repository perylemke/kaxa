package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

const cobraCmdCreate = "create"

func TestSkipConfigLoadNamedCommands(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"help", "completion", "version", "config", commandInit} {
		cmd := &cobra.Command{Use: name}
		if !skipConfigLoad(cmd) {
			t.Fatalf("%q: skip = false", name)
		}
	}
}

func TestSkipConfigLoadHelpFlag(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	cmd.Flags().Bool("help", false, "")
	if err := cmd.Flags().Set("help", "true"); err != nil {
		t.Fatal(err)
	}
	if !skipConfigLoad(cmd) {
		t.Fatal("expected skip when --help")
	}
}

func TestSkipConfigLoadDefaultFalse(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: cobraCmdCreate}
	if skipConfigLoad(cmd) {
		t.Fatal("create should load config")
	}
}
