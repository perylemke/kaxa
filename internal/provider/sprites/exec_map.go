package sprites

import (
	"errors"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func mapExecErr(err error, token string) error {
	if err == nil {
		return nil
	}
	if exitErr, ok := errors.AsType[*spritesdk.ExitError](err); ok {
		return &sandbox.ExitError{Code: exitErr.ExitCode()}
	}
	return redactErr(err, token)
}
