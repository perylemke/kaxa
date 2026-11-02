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
	cmd := &cobra.Command{
		Use:   commandMCP,
		Short: "Serve sandboxes to AI agents over MCP",
		Long: `Serve sandboxes to AI agents, or install that server into an agent.

kaxa mcp and kaxa mcp start serve the Model Context Protocol on stdin and stdout. kaxa mcp init writes the client entry. kaxa mcp config prints it.

Tools take a provider argument, so one workflow works with every provider. Without it, KAXA_DEFAULT_PROVIDER (or --provider) is used. Tokens come from KAXA_* environment variables, never from tool arguments.

Stdout carries only the protocol. Progress and logs go to stderr. Sync copies only a Git repository that contains the current directory or sits below it.`,
		Example: `  kaxa mcp
  kaxa mcp start
  kaxa mcp init cursor
  kaxa mcp config`,
		Args: noArgs(commandMCP),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runMCP(cmd, reg)
		},
	}
	cmd.AddCommand(newMCPStartCmd(reg), newMCPInitCmd(), newMCPConfigCmd())
	return cmd
}

func newMCPStartCmd(reg *registry.Registry) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Serve sandboxes to AI agents over MCP",
		Long: `Serve the Model Context Protocol on stdin and stdout. kaxa mcp with no subcommand does the same.

Stdout carries only the protocol. Progress and logs go to stderr. An MCP client should run this command.`,
		Example: `  kaxa mcp start`,
		Args:    noArgs("mcp start"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runMCP(cmd, reg)
		},
	}
}

func runMCP(cmd *cobra.Command, reg *registry.Registry) error {
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
}
