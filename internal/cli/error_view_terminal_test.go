//go:build unix

package cli

import (
	"strings"
	"testing"

	"github.com/creack/pty"

	"github.com/perylemke/kaxa/internal/usererr"
)

func TestWriteErrorColorTTY(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = ptmx.Close() }()
	defer func() { _ = tty.Close() }()

	t.Setenv("NO_COLOR", "")

	err = usererr.New("boom", "fix it")
	if err := WriteError(tty, err); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 4096)
	n, _ := ptmx.Read(buf)
	out := string(buf[:n])
	if !strings.Contains(out, "boom") || !strings.Contains(out, "fix it") {
		t.Fatalf("out = %q", out)
	}
}

func TestWriteSuccessColorTTY(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = ptmx.Close() }()
	defer func() { _ = tty.Close() }()

	if err := WriteSuccess(tty, "ok", "done"); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 4096)
	n, _ := ptmx.Read(buf)
	out := string(buf[:n])
	if !strings.Contains(out, "ok") || !strings.Contains(out, "done") {
		t.Fatalf("out = %q", out)
	}
}
