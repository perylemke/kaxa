package sandbox_test

import (
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestCappedBufferKeepsOnlyTheLimit(t *testing.T) {
	t.Parallel()

	b := sandbox.NewCappedBuffer(5)
	for _, chunk := range []string{"ab", "cdef", "ghi"} {
		n, err := b.Write([]byte(chunk))
		if err != nil || n != len(chunk) {
			t.Fatalf("Write(%q) = %d, %v", chunk, n, err)
		}
	}
	if got := b.String(); got != "abcde" {
		t.Fatalf("got %q", got)
	}
}

func TestCappedBufferSurvivesHugeOutput(t *testing.T) {
	t.Parallel()

	b := sandbox.NewCappedBuffer(sandbox.MaxCapturedOutput)
	chunk := []byte(strings.Repeat("x", 1<<16))
	for range 64 {
		if _, err := b.Write(chunk); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if len(b.String()) != sandbox.MaxCapturedOutput {
		t.Fatalf("len = %d", len(b.String()))
	}
}
