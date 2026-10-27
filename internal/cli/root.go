// Package cli builds the kaxa command tree.
package cli

import (
	"errors"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/config"
	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
)

// Execute runs the root command and returns an exit code.
func Execute(reg *registry.Registry) int {
	cmd := NewRootCmd(reg)
	if err := cmd.Execute(); err != nil {
		if exitErr, ok := errors.AsType[*sandbox.ExitError](err); ok {
			return remoteExitCode(exitErr.Code)
		}
		_ = WriteError(os.Stderr, lifecycle.WithAlternatives(reg, err))
		return 1
	}
	return 0
}

// NewRootCmd builds the kaxa CLI root command.
func NewRootCmd(reg *registry.Registry) *cobra.Command {
	var provider string

	root := &cobra.Command{
		Use:   "kaxa",
		Short: "Unified CLI for sandbox providers",
		Long: strings.TrimSpace(`
kaxa aggregates sandbox providers behind one CLI.

Supported providers: ` + strings.Join(reg.Names(), ", ") + `.

Configure Sprites or Modal credentials via KAXA_* environment variables (see README).`),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if skipConfigLoad(cmd) {
				return nil
			}
			cfg, err := config.Load(config.LoadOptions{
				Provider: provider,
			}, reg)
			if err != nil {
				return err
			}
			cmd.SetContext(withConfig(cmd.Context(), cfg))
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	root.PersistentFlags().StringVar(&provider, "provider", "", "sandbox provider (sprites, modal)")
	root.SetUsageTemplate(usageTemplate)

	root.AddCommand(newVersionCmd())
	root.AddCommand(newCreateCmd(reg))
	root.AddCommand(newListCmd(reg))
	root.AddCommand(newExecCmd(reg))
	root.AddCommand(newConsoleCmd(reg))
	root.AddCommand(newSyncCmd(reg))
	root.AddCommand(newDestroyCmd(reg))
	root.AddCommand(newRestartCmd(reg))
	root.AddCommand(newUpgradeCmd(reg))
	root.AddCommand(newURLCmd(reg))
	root.AddCommand(newMCPCmd(reg))

	return root
}

func remoteExitCode(code int) int {
	if code < 0 || code > 255 {
		return 1
	}
	return code
}
