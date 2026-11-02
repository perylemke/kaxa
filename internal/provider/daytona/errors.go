package daytona

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

var (
	// ErrMissingCredentials indicates the Daytona API key is not configured.
	ErrMissingCredentials = fmt.Errorf("daytona: missing API key: %w", sandbox.ErrNotConfigured)
	// ErrAlreadyExists is returned when a sandbox name is taken.
	ErrAlreadyExists = errors.New("daytona: sandbox already exists")
	// ErrNotFound is returned when a sandbox does not exist.
	ErrNotFound = errors.New("daytona: sandbox not found")
	// ErrImageNotFound is returned when an image does not exist.
	ErrImageNotFound = errors.New("daytona: image not found")
)

// APIError is a failed Daytona API or toolbox call. Message is the text the
// server sent, never the request, so it carries no credentials.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("daytona: API returned status %d", e.Status)
	}
	return fmt.Sprintf("daytona: API returned status %d: %s", e.Status, e.Message)
}

func missingCredentialsErr() error {
	return usererr.Wrap(ErrMissingCredentials, "missing Daytona API key", "set KAXA_DAYTONA_API_KEY")
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
			"run kaxa list --provider daytona to see the sandboxes")
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
		return usererr.Wrap(redact(api, secrets...), "Daytona rejected the API key", "check KAXA_DAYTONA_API_KEY and its permissions")
	case http.StatusTooManyRequests:
		return usererr.Wrap(redact(api, secrets...), "Daytona rate limit reached", "wait a moment and try again")
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

func notFound(err error) error {
	if api, ok := errors.AsType[*APIError](err); ok && api.Status == http.StatusNotFound {
		return ErrNotFound
	}
	return err
}
