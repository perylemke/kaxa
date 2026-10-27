package cli

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

func newExecCmd(reg *registry.Registry) *cobra.Command {
	return &cobra.Command{
		Use:   "exec <name> <command> [args...]",
		Short: "Run a command in a sandbox",
		Long:  "Runs the command you pass. The remote exit code is the kaxa exit code. For an interactive shell, use kaxa console.",
		Args:  requireExecCommand,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := sandbox.ExecOptions{
				Command: args[1:],
				Stdin:   os.Stdin,
				Stdout:  cmd.OutOrStdout(),
				Stderr:  cmd.ErrOrStderr(),
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGTERM)
			defer stop()

			p, err := providerFromCmd(cmd, reg)
			if err != nil {
				return err
			}
			return p.Exec(ctx, sandbox.ID(args[0]), opts)
		},
	}
}

func requireExecCommand(_ *cobra.Command, args []string) error {
	if len(args) < 2 {
		return usererr.New(
			"exec requires a command",
			"use kaxa console for an interactive shell",
		)
	}
	return nil
}
