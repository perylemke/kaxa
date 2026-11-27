package vercel

import (
	"context"
	"slices"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
)

const (
	// sessionTimeout is how long a session runs before Vercel stops it.
	sessionTimeout = 30 * time.Minute
	// renewBelow is the remaining time under which a command extends the session.
	renewBelow     = 10 * time.Minute
	cleanupTimeout = 15 * time.Second
)

// session is a sandbox with a running session, ready to run commands.
type session struct {
	api     apiClient
	rec     Record
	secrets []string
}

// open finds the sandbox, resumes it when its session stopped, and extends the
// session when little time is left.
func (p *Provider) open(ctx context.Context, id sandbox.ID) (*session, error) {
	if err := validateName(string(id)); err != nil {
		return nil, err
	}
	c, secrets, err := p.clients()
	if err != nil {
		return nil, err
	}
	rec, err := c.Get(ctx, string(id), true)
	if err != nil {
		return nil, userErr(err, string(id), secrets...)
	}
	if rec.SessionID == "" || rec.Status != statusRunning {
		return nil, userErr(errNoSession, string(id), secrets...)
	}
	if !rec.ExpiresAt.IsZero() && time.Until(rec.ExpiresAt) < renewBelow {
		if err := c.ExtendTimeout(ctx, rec.SessionID, sessionTimeout); err != nil {
			return nil, userErr(err, string(id), secrets...)
		}
	}
	return &session{api: c, rec: rec, secrets: secrets}, nil
}

func (s *session) err(err error) error {
	return userErr(err, s.rec.Name, s.secrets...)
}

func (s *session) synced() bool {
	return slices.Contains(s.rec.Labels, sandbox.SyncLabel)
}
