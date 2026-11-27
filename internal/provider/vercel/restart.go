package vercel

import (
	"context"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	defaultReadyTimeout  = 90 * time.Second
	defaultReadyInterval = time.Second
)

// SetReadiness sets how long a stop or an API start waits and how often it
// checks. Tests use it to avoid long waits.
func (p *Provider) SetReadiness(timeout, interval time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.readyTimeout, p.readyInterval = timeout, interval
}

func (p *Provider) readiness() (timeout, interval time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	timeout, interval = p.readyTimeout, p.readyInterval
	if timeout <= 0 {
		timeout = defaultReadyTimeout
	}
	if interval <= 0 {
		interval = defaultReadyInterval
	}
	return timeout, interval
}

// Restart reboots a sandbox from its disk. Vercel has no restart, so kaxa
// stops the session and resumes the name: files survive, but running
// processes, including a published API, do not.
func (p *Provider) Restart(ctx context.Context, id sandbox.ID) error {
	if err := validateName(string(id)); err != nil {
		return err
	}
	c, secrets, err := p.clients()
	if err != nil {
		return err
	}
	_, err = p.cycle(ctx, c, string(id), nil)
	return userErr(err, string(id), secrets...)
}

// stop ends the running session of name, if any, and waits until it is stopped.
func (p *Provider) stop(ctx context.Context, c apiClient, name string) error {
	rec, err := c.Get(ctx, name, false)
	if err != nil {
		return err
	}
	if rec.Status == statusRunning && rec.SessionID != "" {
		if err := c.StopSession(ctx, rec.SessionID); err != nil {
			return err
		}
	}
	timeout, interval := p.readiness()
	deadline := time.Now().Add(timeout)
	for rec.Status != statusStopped {
		if !time.Now().Before(deadline) {
			return usererr.New("the sandbox did not stop in time", "try again in a moment")
		}
		if err := sleep(ctx, interval); err != nil {
			return err
		}
		if rec, err = c.Get(ctx, name, false); err != nil {
			return err
		}
	}
	return nil
}

// cycle stops the session of name, runs between while it is stopped, and
// resumes it. between may be nil.
func (p *Provider) cycle(ctx context.Context, c apiClient, name string, between func() error) (Record, error) {
	if err := p.stop(ctx, c, name); err != nil {
		return Record{}, err
	}
	if between != nil {
		if err := between(); err != nil {
			return Record{}, err
		}
	}
	return c.Get(ctx, name, true)
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
