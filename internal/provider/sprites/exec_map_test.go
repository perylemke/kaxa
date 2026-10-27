package sprites

import (
	"errors"
	"testing"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestMapExecErrExitCode(t *testing.T) {
	t.Parallel()

	err := mapExecErr(&spritesdk.ExitError{Code: 7}, "")
	var exit *sandbox.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("want ExitError, got %T", err)
	}
	if exit.Code != 7 {
		t.Fatalf("code = %d", exit.Code)
	}
}
