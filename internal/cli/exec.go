package cli

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/perylemke/sbox/internal/registry"
	"github.com/perylemke/sbox/internal/sandbox"
)

func newExecCmd(reg *registry.Registry) *cobra.Command {
	return &cobra.Command{
		Use:   "exec <name> [command...]",
		Short: "Run a command or interactive shell in a sandbox",
		Long:  "With no command, opens an interactive bash shell (requires a terminal).",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			interactive := len(args) == 1
			command := args[1:]
			if interactive {
				command = []string{"bash"}
			}

			opts := sandbox.ExecOptions{
				Command: command,
				Stdin:   os.Stdin,
				Stdout:  cmd.OutOrStdout(),
				Stderr:  cmd.ErrOrStderr(),
				TTY:     interactive,
			}

			ctx := cmd.Context()
			if interactive {
				var cleanup func()
				var err error
				ctx, cleanup, err = wrapInteractive(ctx, &opts)
				if err != nil {
					return err
				}
				defer cleanup()
			} else {
				var stop func()
				ctx, stop = signal.NotifyContext(ctx, syscall.SIGTERM)
				defer stop()
			}

			p, err := providerFromCmd(cmd, reg)
			if err != nil {
				return err
			}
			return p.Exec(ctx, sandbox.ID(name), opts)
		},
	}
}

func wrapInteractive(ctx context.Context, opts *sandbox.ExecOptions) (context.Context, func(), error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return ctx, func() {}, errors.New("interactive shell requires a terminal (run from a TTY)")
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
