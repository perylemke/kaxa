//go:build unix

package cli

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"golang.org/x/term"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestStartWinchWatcher(t *testing.T) {
	fd := int(os.Stdout.Fd())
	if !term.IsTerminal(fd) {
		t.Skip("stdout is not a terminal")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ch := make(chan sandbox.TermSize, 1)
	startWinchWatcher(ctx, fd, ch)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGWINCH); err != nil {
		t.Fatalf("signal: %v", err)
	}
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for resize")
	}
}
