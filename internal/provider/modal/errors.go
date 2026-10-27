package modal

import (
	"errors"
	"fmt"
	"strings"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

var (
	// ErrMissingCredentials indicates the Modal token ID or secret is not configured.
	ErrMissingCredentials = fmt.Errorf("modal: missing API credentials: %w", sandbox.ErrNotConfigured)
	// ErrAlreadyExists is returned by a client when a sandbox name is taken.
	ErrAlreadyExists = errors.New("modal: sandbox already exists")
	// ErrNotFound is returned by a client when a sandbox does not exist.
	ErrNotFound = errors.New("modal: sandbox not found")
)

func missingCredentialsErr() error {
	return usererr.Wrap(
		ErrMissingCredentials,
		"missing Modal API credentials",
		"set KAXA_MODAL_TOKEN_ID and KAXA_MODAL_TOKEN_SECRET",
	)
}

// userErr turns client errors into user-facing errors and keeps credentials
// out of any message.
func userErr(err error, name string, secrets ...string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrAlreadyExists):
		return usererr.Wrap(err, fmt.Sprintf("sandbox %q already exists", name),
			"choose another name or destroy the existing sandbox")
	case errors.Is(err, ErrNotFound):
		return usererr.Wrap(err, fmt.Sprintf("sandbox %q not found", name),
			"run kaxa list --provider modal to see the sandboxes")
	}
	return redact(err, secrets...)
}

func redact(err error, secrets ...string) error {
	msg := err.Error()
	redacted := msg
	for _, s := range secrets {
		if s != "" {
			redacted = strings.ReplaceAll(redacted, s, "<redacted>")
		}
	}
	if redacted == msg {
		return err
	}
	return errors.New(redacted)
}
