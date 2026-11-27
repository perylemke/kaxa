package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

// exitInterrupted is the exit code of a command stopped by Ctrl+C, as in a shell.
const exitInterrupted = 130

// signalContext returns a context that ends on the first SIGINT or SIGTERM.
//
// The first signal prints a notice and lets the command clean up. It also
// restores the default handling, so a second Ctrl+C ends the process at once.
// Call the returned function when the command is done.
func signalContext(parent context.Context, errOut io.Writer) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	go func() {
		defer signal.Stop(sigs)
		select {
		case <-sigs:
			_, _ = fmt.Fprintln(errOut, "Interrupted, shutting down… (Ctrl+C again to force)")
			// Stop first, so the default handling ends the process on a second signal.
			signal.Stop(sigs)
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}
