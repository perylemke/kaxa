package sprites

import (
	"errors"
	"strings"
)

// ErrMissingToken indicates Sprites credentials were not configured.
var ErrMissingToken = errors.New("sprites: missing API token (set SBOX_SPRITES_TOKEN)")

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
