package docker

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

var (
	// ErrMissingCredentials indicates the Docker ID or access token is not configured.
	ErrMissingCredentials = fmt.Errorf("docker: missing Docker ID or personal access token: %w", sandbox.ErrNotConfigured)
	// ErrAlreadyExists is returned when a sandbox name is taken.
	ErrAlreadyExists = errors.New("docker: sandbox already exists")
	// ErrNotFound is returned when a sandbox does not exist.
	ErrNotFound = errors.New("docker: sandbox not found")
	// ErrSnapshotNotFound is returned when a snapshot does not exist.
	ErrSnapshotNotFound = errors.New("docker: snapshot not found")
	// ErrSnapshotExists is returned when a snapshot display name is taken.
	ErrSnapshotExists = errors.New("docker: snapshot already exists")
)

// Error codes of the Sandboxes API that kaxa tells apart.
const (
	codeNotFound         = "notFound"
	codeAlreadyExists    = "alreadyExists"
	codeUnauthenticated  = "unauthenticated"
	codePermissionDenied = "permissionDenied"
	codeExhausted        = "resourceExhausted"
	codeUnimplemented    = "unimplemented"
)

// APIError is a failed Sandboxes API call. Code and Message are what the
// server sent, never the request, so they carry no credentials.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	switch {
	case e.Code != "" && e.Message != "":
		return fmt.Sprintf("docker: %s: %s", e.Code, e.Message)
	case e.Code != "":
		return "docker: " + e.Code
	case e.Message != "":
		return fmt.Sprintf("docker: API returned status %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("docker: API returned status %d", e.Status)
}

func (e *APIError) is(code string, status int) bool {
	return e.Code == code || (e.Code == "" && e.Status == status)
}

func missingCredentialsErr() error {
	return usererr.Wrap(
		ErrMissingCredentials,
		"missing Docker credentials",
		"set KAXA_DOCKER_ID and KAXA_DOCKER_PAT (a personal access token with Allow Cloud Sandboxes access)",
	)
}

// userErr turns client errors into user-facing errors and keeps credentials
// out of any message.
func userErr(err error, name string, secrets ...string) error {
	if err == nil {
		return nil
	}
	if ue, ok := errors.AsType[*usererr.Error](err); ok {
		return ue
	}
	if errors.Is(err, ErrAlreadyExists) {
		return usererr.Wrap(err, fmt.Sprintf("sandbox %q already exists", name),
			"choose another name or destroy the existing sandbox")
	}
	if errors.Is(err, ErrNotFound) {
		return usererr.Wrap(err, fmt.Sprintf("sandbox %q not found", name),
			"run kaxa list to see the sandboxes kaxa created")
	}
	if apiErr, ok := errors.AsType[*APIError](err); ok {
		if mapped := mapAPIError(apiErr, secrets); mapped != nil {
			return mapped
		}
	}
	msg := err.Error()
	if clean := redact(msg, secrets); clean != msg {
		return &redactedError{msg: clean, err: err}
	}
	return err
}

// redactedError shows a message without credentials and still unwraps to the
// original error, so errors.Is and errors.As keep working.
type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }

// Unwrap returns the original error.
func (e *redactedError) Unwrap() error { return e.err }

func mapAPIError(e *APIError, secrets []string) error {
	switch {
	case e.is(codeUnauthenticated, http.StatusUnauthorized):
		return usererr.Wrap(e, "Docker rejected the credentials",
			"check KAXA_DOCKER_ID and KAXA_DOCKER_PAT, and that the token was created with Allow Cloud Sandboxes access")
	case e.is(codePermissionDenied, http.StatusForbidden):
		return usererr.Wrap(e, "Docker denied access to Cloud Sandboxes",
			"the account needs an active Docker Agentic Platform subscription and a token with the sandbox:use scope")
	case e.Code == codeExhausted || e.Status == http.StatusTooManyRequests:
		return usererr.Wrap(e, "Docker Cloud Sandboxes limit reached: "+redact(e.Message, secrets),
			"destroy sandboxes you no longer need, or wait and try again; by default an account has 10 running and 50 stored sandboxes")
	}
	return nil
}

// redact removes every secret from text.
func redact(text string, secrets []string) string {
	for _, s := range secrets {
		if s != "" {
			text = strings.ReplaceAll(text, s, "[redacted]")
		}
	}
	return text
}
