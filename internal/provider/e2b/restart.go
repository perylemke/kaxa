package e2b

import (
	"context"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// Restart reboots a sandbox from its disk. E2B has no restart, so kaxa pauses
// the sandbox without its memory and resumes it: files survive, but running
// processes, including a published API, do not.
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
	if rec.State != statePaused {
		if err := c.Pause(ctx, rec.ID, false); err != nil {
			return userErr(err, string(id), secrets...)
		}
	}
	if _, err := c.Connect(ctx, rec.ID, ConnectParams{Reboot: true}); err != nil {
		return userErr(err, string(id), secrets...)
	}
	return nil
}
