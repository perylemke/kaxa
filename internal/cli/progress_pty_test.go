//go:build unix

package cli

import (
	"context"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestRunTaskWithSpinnerTTYUsesFrames(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = ptmx.Close() }()
	defer func() { _ = tty.Close() }()

	errCh := make(chan error, 1)
	go func() {
		errCh <- runTaskWithSpinner(context.Background(), tty, "label", func(context.Context) error {
			return nil
		})
	}()

	buf := make([]byte, 64)
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for spinner output")
		default:
		}
		n, err := ptmx.Read(buf)
		if err == nil && n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}
