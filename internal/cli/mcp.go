package cli

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/mcp"
	"github.com/perylemke/kaxa/internal/registry"
)

func newMCPCmd(reg *registry.Registry) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve sandboxes to AI agents over MCP",
		Long: `Run a Model Context Protocol server on stdin and stdout so an AI agent can create sandboxes, run commands in them and destroy them.

Tools take a provider argument, so one workflow works with every provider. Without it, KAXA_DEFAULT_PROVIDER (or --provider) is used. Tokens come from KAXA_* environment variables, never from tool arguments.

Stdout carries only the protocol. Progress and logs go to stderr. Sync copies only a Git repository that contains the current directory or sits below it.`,
		Example: `  kaxa mcp`,
		Args:    noArgs("mcp"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := configFromContext(cmd.Context())
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			err = mcp.Serve(ctx, reg, cfg, cmd.ErrOrStderr())
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		},
	}
}
