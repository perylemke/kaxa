package e2b

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

var (
	// ErrMissingCredentials indicates the E2B API key is not configured.
	ErrMissingCredentials = fmt.Errorf("e2b: missing API key: %w", sandbox.ErrNotConfigured)
	// ErrAlreadyExists is returned by a client when a sandbox name is taken.
	ErrAlreadyExists = errors.New("e2b: sandbox already exists")
	// ErrNotFound is returned by a client when a sandbox does not exist.
	ErrNotFound = errors.New("e2b: sandbox not found")
	// ErrTemplateNotFound is returned by a client when the template does not exist.
	ErrTemplateNotFound = errors.New("e2b: template not found")
)

// APIError is a failed E2B API or envd call. Message is the text the server
// sent, never the request, so it carries no credentials. Status is zero for an
// error the sandbox daemon reported inside a stream.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Status == 0 {
		return "e2b: " + e.Message
	}
	if e.Message == "" {
		return fmt.Sprintf("e2b: API returned status %d", e.Status)
	}
	return fmt.Sprintf("e2b: API returned status %d: %s", e.Status, e.Message)
}

func missingCredentialsErr() error {
	return usererr.Wrap(
		ErrMissingCredentials,
		"missing E2B API key",
		"set KAXA_E2B_API_KEY",
	)
}

// userErr turns client errors into user-facing errors and keeps credentials
// out of any message.
func userErr(err error, name string, secrets ...string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrAlreadyExists) {
		return usererr.Wrap(err, fmt.Sprintf("sandbox %q already exists", name),
			"choose another name or destroy the existing sandbox")
	}
	if errors.Is(err, ErrNotFound) {
		return usererr.Wrap(err, fmt.Sprintf("sandbox %q not found", name),
			"run kaxa list --provider e2b to see the sandboxes")
	}
	if api, ok := errors.AsType[*APIError](err); ok {
		if ue := apiUserErr(api, secrets); ue != nil {
			return ue
		}
	}
	return redact(err, secrets...)
}

func apiUserErr(api *APIError, secrets []string) error {
	switch api.Status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return usererr.Wrap(redact(api, secrets...), "E2B rejected the API key", "check KAXA_E2B_API_KEY")
	case http.StatusTooManyRequests:
		return usererr.Wrap(redact(api, secrets...), "E2B rate limit reached", "wait a moment and try again")
	}
	return nil
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
