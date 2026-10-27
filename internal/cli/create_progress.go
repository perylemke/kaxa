package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
)

func runTaskWithSpinner(ctx context.Context, errOut io.Writer, label string, task func(context.Context) error) error {
	if !listOutputIsTTY(errOut) {
		return task(ctx)
	}

	done := make(chan error, 1)
	go func() {
		done <- task(ctx)
	}()

	frames := spinner.Line.Frames
	interval := spinner.Line.FPS
	if interval <= 0 {
		interval = time.Second / 10
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	frame := 0
	clearLine := func() {
		_, _ = fmt.Fprint(errOut, "\r\033[K")
	}

	for {
		select {
		case err := <-done:
			clearLine()
			return err
		case <-ctx.Done():
			clearLine()
			return ctx.Err()
		case <-ticker.C:
			if len(frames) == 0 {
				continue
			}
			_, _ = fmt.Fprintf(errOut, "\r%s %s", frames[frame], label)
			frame = (frame + 1) % len(frames)
		}
	}
}
