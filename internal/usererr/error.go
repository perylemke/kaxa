// Package usererr holds CLI-facing errors with optional hints.
package usererr

import "errors"

// Error is a user-facing error with an optional next step.
type Error struct {
	Message string
	Hint    string
	Cause   error
}

func (e *Error) Error() string {
	return e.Message
}

// Unwrap returns the underlying cause for errors.Is and errors.As.
func (e *Error) Unwrap() error {
	return e.Cause
}

// New returns an error with message and hint.
func New(message, hint string) error {
	return &Error{Message: message, Hint: hint}
}

// Wrap returns an error with message and hint, wrapping cause.
func Wrap(cause error, message, hint string) error {
	return &Error{Message: message, Hint: hint, Cause: cause}
}

// As extracts a usererr.Error from err.
func As(err error) (*Error, bool) {
	ue, ok := errors.AsType[*Error](err)
	return ue, ok
}
