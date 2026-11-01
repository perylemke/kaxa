package daytona

import (
	"context"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// Restart reboots a sandbox from its disk. Daytona stops the sandbox, which
// keeps the disk, and starts it again: files survive, but running processes,
// including a published API, do not.
func (p *Provider) Restart(ctx context.Context, id sandbox.ID) error {
	if err := validateName(string(id)); err != nil {
		return err
	}
	c, _, secrets, err := p.clients()
	if err != nil {
		return err
	}
	rec, err := c.Find(ctx, string(id))
	if err != nil {
		return userErr(err, string(id), secrets...)
	}
	if rec.Status == statusRunning {
		if err := c.Stop(ctx, rec.ID); err != nil {
			return userErr(err, string(id), secrets...)
		}
		if rec, err = p.waitStatus(ctx, c, rec, statusStopped); err != nil {
			return userErr(err, string(id), secrets...)
		}
	}
	if _, err := p.ensureRunning(ctx, c, rec); err != nil {
		return userErr(err, string(id), secrets...)
	}
	return nil
}
