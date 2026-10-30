package docker

import (
	"context"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// Restart stops a sandbox and starts it again. Files survive; running
// processes do not.
func (p *Provider) Restart(ctx context.Context, id sandbox.ID) error {
	api, rec, err := p.find(ctx, id)
	if err != nil {
		return err
	}
	// A fresh read gives the current status and etag, which Docker checks.
	rec, err = api.Get(ctx, rec.Resource)
	if err != nil {
		return p.fail(api, err, string(id))
	}
	if rec.Status != statusStopped {
		if err := api.Stop(ctx, rec); err != nil {
			return p.fail(api, err, string(id))
		}
		if rec, err = p.waitStopped(ctx, api, rec); err != nil {
			return p.fail(api, err, string(id))
		}
	}
	if err := api.Start(ctx, rec); err != nil {
		return p.fail(api, err, string(id))
	}
	if _, err := p.waitRunning(ctx, api, rec); err != nil {
		return p.fail(api, err, string(id))
	}
	return nil
}
