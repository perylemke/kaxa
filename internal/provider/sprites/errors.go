package sprites

import (
	"errors"
	"fmt"
	"strings"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

// ErrMissingToken indicates Sprites credentials were not configured.
var ErrMissingToken = fmt.Errorf("sprites: missing API token: %w", sandbox.ErrNotConfigured)

func missingTokenErr() error {
	return usererr.Wrap(ErrMissingToken, "missing Sprites API token", "set KAXA_SPRITES_TOKEN")
}

func redactErr(err error, token string) error {
	if err == nil {
		return nil
	}
	if token == "" {
		return err
	}
	msg := err.Error()
	if !strings.Contains(msg, token) {
		return err
	}
	return errors.New(strings.ReplaceAll(msg, token, "<redacted>"))
}
