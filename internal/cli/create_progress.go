package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/charmbracelet/bubbles/spinner"

	"github.com/perylemke/sbox/internal/sandbox"
)

func runCreateWithSpinner(ctx context.Context, errOut io.Writer, name string, create func(context.Context) (sandbox.ID, error)) (sandbox.ID, error) {
	if !listOutputIsTTY(errOut) {
		return create(ctx)
	}

	type result struct {
		id  sandbox.ID
		err error
	}
	done := make(chan result, 1)
	go func() {
		id, err := create(ctx)
		done <- result{id: id, err: err}
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
		case res := <-done:
			clearLine()
			return res.id, res.err
		case <-ctx.Done():
			clearLine()
			return "", ctx.Err()
		case <-ticker.C:
			if len(frames) == 0 {
				continue
			}
			_, _ = fmt.Fprintf(errOut, "\r%s creating %s…", frames[frame], name)
			frame = (frame + 1) % len(frames)
		}
	}
}
