package vercel

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

var (
	// ErrMissingCredentials indicates the Vercel token or project is not configured.
	ErrMissingCredentials = fmt.Errorf("vercel: missing token or project: %w", sandbox.ErrNotConfigured)
	// ErrAlreadyExists is returned by a client when a sandbox name is taken.
	ErrAlreadyExists = errors.New("vercel: sandbox already exists")
	// ErrNotFound is returned by a client when a sandbox or snapshot does not exist.
	ErrNotFound = errors.New("vercel: not found")
)

// APIError is a failed Vercel API call. Message is the text the server sent,
// never the request, so it carries no credentials.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("vercel: API returned status %d", e.Status)
	}
	return fmt.Sprintf("vercel: API returned status %d: %s", e.Status, e.Message)
}

var errNoSession = usererr.New(
	"the sandbox has no running session",
	"try again in a moment, or check the sandbox in the Vercel dashboard",
)

func missingCredentialsErr() error {
	return usererr.Wrap(
		ErrMissingCredentials,
		"missing Vercel token or project",
		"set KAXA_VERCEL_TOKEN and KAXA_VERCEL_PROJECT (and KAXA_VERCEL_TEAM for a team)",
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
		return usererr.Wrap(err, fmt.Sprintf("%q not found", name),
			"run kaxa list --provider vercel to see the sandboxes")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return usererr.Wrap(errors.New("vercel: request timed out"),
			fmt.Sprintf("vercel is still starting sandbox %q", name),
			"run kaxa list --provider vercel in a minute and retry; a snapshot that is not cached can take several minutes")
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
		return usererr.Wrap(redact(api, secrets...), "Vercel rejected the credentials",
			"check KAXA_VERCEL_TOKEN, KAXA_VERCEL_PROJECT, and KAXA_VERCEL_TEAM")
	case http.StatusPaymentRequired:
		return usererr.Wrap(redact(api, secrets...), "Vercel refused the request because of a plan or billing limit",
			"check the plan of the team in the Vercel dashboard")
	case http.StatusTooManyRequests:
		return usererr.Wrap(redact(api, secrets...), "Vercel rate limit reached", "wait a moment and try again")
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
