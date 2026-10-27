//go:build unix

package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func startWinchWatcher(ctx context.Context, fd int, resize chan<- sandbox.TermSize) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		defer signal.Stop(sigCh)
		for {
			select {
			case <-ctx.Done():
				return
			case <-sigCh:
				w, h, err := term.GetSize(fd)
				if err != nil {
					continue
				}
				size := sandbox.TermSize{Rows: termRows(h), Cols: termCols(w)}
				select {
				case resize <- size:
				default:
				}
			}
		}
	}()
}
