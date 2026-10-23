package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/perylemke/sbx/internal/config"
	"github.com/perylemke/sbx/internal/registry"
)

// Execute runs the root command and returns an exit code.
func Execute(reg *registry.Registry) int {
	cmd := NewRootCmd(reg)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

// NewRootCmd builds the sbx CLI root command.
func NewRootCmd(reg *registry.Registry) *cobra.Command {
	var configPath string
	var provider string

	root := &cobra.Command{
		Use:   "sbx",
		Short: "Unified CLI for sandbox providers",
		Long: strings.TrimSpace(`
sbx aggregates sandbox providers behind one CLI.

Supported providers: ` + strings.Join(reg.Names(), ", ") + `.

Configure Sprites or Modal credentials via config file or SBX_* environment variables.
See config.example.yaml in the repository root.`),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Name() == "help" || cmd.Name() == "completion" || cmd.Name() == "version" {
				return nil
			}
			_, err := config.Load(config.LoadOptions{
				ConfigPath: configPath,
				Provider:   provider,
			}, reg)
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	root.PersistentFlags().StringVar(&configPath, "config", "", "config file path")
	root.PersistentFlags().StringVar(&provider, "provider", "", "sandbox provider (sprites, modal)")

	root.AddCommand(newVersionCmd())

	return root
}
