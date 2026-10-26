package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/perylemke/sbox/internal/registry"
	"github.com/perylemke/sbox/internal/sandbox"
	"github.com/perylemke/sbox/internal/usererr"
)

func newConsoleCmd(reg *registry.Registry) *cobra.Command {
	return &cobra.Command{
		Use:     "console <name>",
		Aliases: []string{"ssh"},
		Short:   "Open an interactive shell in a sandbox",
		Long:    "Opens an interactive bash shell. Requires a terminal.",
		Args:    requireConsoleName,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := sandbox.ExecOptions{
				Command: []string{"bash"},
				Stdin:   os.Stdin,
				Stdout:  cmd.OutOrStdout(),
				Stderr:  cmd.ErrOrStderr(),
				TTY:     true,
			}

			ctx, cleanup, err := wrapInteractive(cmd.Context(), &opts)
			if err != nil {
				return err
			}
			defer cleanup()

			p, err := providerFromCmd(cmd, reg)
			if err != nil {
				return err
			}
			return p.Exec(ctx, sandbox.ID(args[0]), opts)
		},
	}
}

func requireConsoleName(_ *cobra.Command, args []string) error {
	if len(args) == 1 {
		return nil
	}
	if len(args) == 0 {
		return usererr.New(
			"console requires a sandbox name",
			"sbox console <name>",
		)
	}
	return usererr.New(
		"console accepts one sandbox name",
		"sbox console <name>",
	)
}

func wrapInteractive(ctx context.Context, opts *sandbox.ExecOptions) (context.Context, func(), error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return ctx, func() {}, usererr.New(
			"interactive shell requires a terminal",
			"run sbox console from a terminal",
		)
	}

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return ctx, func() {}, err
	}

	cleanup := func() {
		_ = term.Restore(fd, oldState)
	}

	w, h, err := term.GetSize(fd)
	if err != nil {
		cleanup()
		return ctx, func() {}, err
	}
	opts.Size = sandbox.TermSize{Rows: termRows(h), Cols: termCols(w)}

	resizeCh := make(chan sandbox.TermSize, 1)
	opts.Resize = resizeCh
	startWinchWatcher(ctx, fd, resizeCh)

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM)
	prevCleanup := cleanup
	cleanup = func() {
		stop()
		prevCleanup()
	}

	return ctx, cleanup, nil
}
