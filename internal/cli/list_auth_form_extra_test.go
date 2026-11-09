package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestCollectURLAuthAccessible(t *testing.T) {
	t.Parallel()

	in := strings.NewReader("1\n")
	out := new(bytes.Buffer)
	got, err := collectURLAuth(in, out, sandbox.URLAuthSprite)
	if err != nil {
		t.Fatal(err)
	}
	if got != sandbox.URLAuthPublic {
		t.Fatalf("choice = %q", got)
	}
}
