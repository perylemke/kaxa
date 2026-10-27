package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/usererr"
)

func TestWriteErrorIncludesMessageAndHint(t *testing.T) {
	t.Parallel()

	buf := new(bytes.Buffer)
	err := usererr.New(`unknown provider "nope"`, "supported: modal, sprites")
	if writeErr := WriteError(buf, err); writeErr != nil {
		t.Fatalf("WriteError: %v", writeErr)
	}

	out := buf.String()
	if strings.Contains(out, "\x1b") {
		t.Fatalf("buffer output should not contain ANSI: %q", out)
	}

	plain := stripANSI(out)
	if !strings.Contains(plain, "unknown provider") {
		t.Fatalf("output = %q", plain)
	}
	if !strings.Contains(plain, "hint") {
		t.Fatalf("output missing hint: %q", plain)
	}
	if !strings.Contains(plain, "supported: modal, sprites") {
		t.Fatalf("output missing hint text: %q", plain)
	}
}

func TestWriteSuccessIncludesReadyMessage(t *testing.T) {
	t.Parallel()

	buf := new(bytes.Buffer)
	if err := WriteSuccess(buf, "created", "sandbox my-box is ready"); err != nil {
		t.Fatalf("WriteSuccess: %v", err)
	}
	plain := stripANSI(buf.String())
	if !strings.Contains(plain, "created") || !strings.Contains(plain, "sandbox my-box is ready") {
		t.Fatalf("output = %q", plain)
	}
	if strings.Contains(buf.String(), "\x1b") {
		t.Fatalf("buffer output should not contain ANSI: %q", buf.String())
	}
}

func TestWriteErrorPlainAPIError(t *testing.T) {
	t.Parallel()

	buf := new(bytes.Buffer)
	err := usererr.New("sprites API request failed", "")
	if err := WriteError(buf, err); err != nil {
		t.Fatalf("WriteError: %v", err)
	}
	plain := stripANSI(buf.String())
	if !strings.Contains(plain, "sprites API request failed") {
		t.Fatalf("output = %q", plain)
	}
	if strings.Contains(plain, "hint") {
		t.Fatalf("unexpected hint line: %q", plain)
	}
}
