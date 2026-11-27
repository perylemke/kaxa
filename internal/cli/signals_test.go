package cli

import (
	"bytes"
	"context"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSignalContextEndsOnASignalAndSaysSo(t *testing.T) {
	var out bytes.Buffer
	ctx, stop := signalContext(t.Context(), &out)
	defer stop()
	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the context did not end on SIGINT")
	}
	if !strings.Contains(out.String(), "Ctrl+C again to force") {
		t.Fatalf("notice = %q", out.String())
	}
}

func TestSignalContextIsQuietWhenTheCommandEnds(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	ctx, stop := signalContext(t.Context(), &out)
	stop()
	<-ctx.Done()
	time.Sleep(20 * time.Millisecond)
	if out.Len() != 0 {
		t.Fatalf("notice = %q, want none", out.String())
	}
}

func TestSignalContextFollowsItsParent(t *testing.T) {
	t.Parallel()

	parent, cancel := context.WithCancel(t.Context())
	ctx, stop := signalContext(parent, &bytes.Buffer{})
	defer stop()
	cancel()
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the context did not follow its parent")
	}
}
