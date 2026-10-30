package toolchain

import (
	"context"
	"strings"
	"time"
)

const projectAttempts = 3

// retryDelay is the pause before attempt n+1 is n times this long.
var retryDelay = 3 * time.Second

// transientMarkers are lowercase fragments of network errors that a second try
// usually fixes, such as a package registry closing an HTTP/2 connection.
var transientMarkers = []string{
	"goaway",
	"connection reset",
	"connection refused",
	"unexpected eof",
	"tls handshake timeout",
	"i/o timeout",
	"timed out",
	"timeout awaiting",
	"temporary failure",
	"try again",
	"bad gateway",
	"service unavailable",
	"gateway timeout",
	"too many requests",
	"status 502",
	"status 503",
	"status 504",
	"econnreset",
	"etimedout",
	"eai_again",
}

// isTransient reports whether err looks like a passing network failure.
func isTransient(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range transientMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// runWithRetry runs a project command up to projectAttempts times while it
// fails with a transient network error. Dependency commands such as go build,
// npm ci, and uv sync are safe to repeat: they resume from their cache.
func (i *installer) runWithRetry(ctx context.Context, dir string, env, argv []string) error {
	var err error
	for attempt := 1; attempt <= projectAttempts; attempt++ {
		if _, err = i.runIn(ctx, dir, env, argv, nil); err == nil || !isTransient(err) {
			return err
		}
		if attempt == projectAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * retryDelay):
		}
	}
	return err
}
