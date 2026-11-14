package blaxel

import (
	"context"
	"fmt"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

// Unarchive brings an archived sandbox back and waits until it can run commands.
// A sandbox that is not archived is left as it is.
func (p *Provider) Unarchive(ctx context.Context, id sandbox.ID) error {
	if err := validateName(string(id)); err != nil {
		return err
	}
	c, _, secrets, err := p.clients()
	if err != nil {
		return err
	}
	rec, err := p.resolve(ctx, c, string(id))
	if err != nil {
		return userErr(err, string(id), secrets...)
	}
	if rec.Status != statusArchived {
		return usererr.New(
			fmt.Sprintf("sandbox %q is %s", rec.Name, rec.Status),
			"unarchive is only for an archived sandbox",
		)
	}
	rec, err = c.Unarchive(ctx, rec.Name)
	if err != nil {
		return userErr(err, string(id), secrets...)
	}
	if _, err := p.waitRunning(ctx, c, rec); err != nil {
		return userErr(err, string(id), secrets...)
	}
	return nil
}
