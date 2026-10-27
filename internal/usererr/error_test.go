package usererr_test

import (
	"errors"
	"testing"

	"github.com/perylemke/kaxa/internal/usererr"
)

func TestWrapUnwrap(t *testing.T) {
	t.Parallel()

	cause := errors.New("cause")
	err := usererr.Wrap(cause, "message", "hint")
	if err.Error() != "message" {
		t.Fatalf("Error() = %q", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Fatal("expected errors.Is to find cause")
	}
	ue, ok := usererr.As(err)
	if !ok || ue.Hint != "hint" {
		t.Fatalf("As = %v, %q", ok, ue)
	}
}
