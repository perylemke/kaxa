package sprites

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

// RestoreMode reports that Sprites restores a checkpoint onto the same sprite.
func (p *Provider) RestoreMode() sandbox.RestoreMode { return sandbox.RestoreInPlace }

// Snapshot saves a checkpoint of a sprite. The create stream does not name the
// new checkpoint, so it is found by listing before and after.
func (p *Provider) Snapshot(ctx context.Context, id sandbox.ID, comment string) (sandbox.Snapshot, error) {
	c, token, err := p.checkpoints(id)
	if err != nil {
		return sandbox.Snapshot{}, err
	}
	if err := sandbox.ValidateSnapshotComment(comment); err != nil {
		return sandbox.Snapshot{}, usererr.Wrap(err, err.Error(), "shorten the comment and remove control characters")
	}
	before, err := listSnapshotCheckpoints(ctx, c, id)
	if err != nil {
		return sandbox.Snapshot{}, redactErr(err, token)
	}
	if err := c.CreateCheckpoint(ctx, string(id), comment); err != nil {
		return sandbox.Snapshot{}, redactErr(err, token)
	}
	after, err := listSnapshotCheckpoints(ctx, c, id)
	if err != nil {
		return sandbox.Snapshot{}, redactErr(err, token)
	}
	fresh := added(before, after)
	if len(fresh) != 1 {
		// Two or none: the new checkpoint cannot be told apart from another
		// create, and a wrong ID would roll the sprite back to the wrong state.
		return sandbox.Snapshot{}, usererr.New(
			fmt.Sprintf("could not tell which checkpoint of %q is the new one (%d appeared)", id, len(fresh)),
			fmt.Sprintf("run kaxa snapshot list %s --provider sprites to find it", id),
		)
	}
	return p.toSnapshot(id, fresh[0]), nil
}

// ListSnapshots returns the checkpoints of a sprite, newest first.
func (p *Provider) ListSnapshots(ctx context.Context, id sandbox.ID) ([]sandbox.Snapshot, error) {
	c, token, err := p.checkpoints(id)
	if err != nil {
		return nil, err
	}
	cps, err := listSnapshotCheckpoints(ctx, c, id)
	if err != nil {
		return nil, redactErr(err, token)
	}
	out := make([]sandbox.Snapshot, 0, len(cps))
	for _, cp := range cps {
		out = append(out, p.toSnapshot(id, cp))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// Restore returns a sprite to a checkpoint. It overwrites the sprite and
// takes no target.
func (p *Provider) Restore(ctx context.Context, req sandbox.RestoreRequest) (sandbox.ID, error) {
	c, token, err := p.checkpoints(req.Sandbox)
	if err != nil {
		return "", err
	}
	if req.Target != "" {
		return "", usererr.New("sprites restores in place and takes no target", "omit the target")
	}
	if err := validateSnapshotID(req.Snapshot); err != nil {
		return "", err
	}
	if err := c.RestoreCheckpoint(ctx, string(req.Sandbox), string(req.Snapshot)); err != nil {
		return "", redactErr(err, token)
	}
	return req.Sandbox, nil
}

// DeleteSnapshot removes a checkpoint.
func (p *Provider) DeleteSnapshot(ctx context.Context, id sandbox.ID, snap sandbox.SnapshotID) error {
	c, token, err := p.checkpoints(id)
	if err != nil {
		return err
	}
	if err := validateSnapshotID(snap); err != nil {
		return err
	}
	err = c.DeleteCheckpoint(ctx, string(id), string(snap))
	if se, ok := errors.AsType[*statusError](err); ok && se.Code == http.StatusNotFound {
		return usererr.Wrap(err, fmt.Sprintf("snapshot %q not found for sandbox %q", snap, id),
			"list the snapshots of the sandbox to see its ids")
	}
	return redactErr(err, token)
}

// checkpoints validates id and returns the client that serves checkpoints.
func (p *Provider) checkpoints(id sandbox.ID) (checkpointClient, string, error) {
	if err := validateName(string(id)); err != nil {
		return nil, "", err
	}
	c, token, err := p.clientAndToken()
	if err != nil {
		return nil, "", err
	}
	cc, ok := c.(checkpointClient)
	if !ok {
		return nil, "", errors.New("sprites: client does not support checkpoints")
	}
	return cc, token, nil
}

// liveCheckpointID is how the API names the running state of a sprite, which
// it lists next to the saved checkpoints.
const liveCheckpointID = "Current"

// listSnapshotCheckpoints lists the saved checkpoints, without the live state.
func listSnapshotCheckpoints(ctx context.Context, c checkpointClient, id sandbox.ID) ([]*spritesdk.Checkpoint, error) {
	cps, err := c.ListCheckpoints(ctx, string(id))
	if err != nil {
		return nil, err
	}
	out := make([]*spritesdk.Checkpoint, 0, len(cps))
	for _, cp := range cps {
		if !strings.EqualFold(cp.ID, liveCheckpointID) {
			out = append(out, cp)
		}
	}
	return out, nil
}

func validateSnapshotID(id sandbox.SnapshotID) error {
	if !sandbox.ValidSnapshotID(id) || strings.EqualFold(string(id), liveCheckpointID) {
		return usererr.New(fmt.Sprintf("invalid snapshot id %q", id), "use an id from the snapshot list, such as v7")
	}
	return nil
}

func (p *Provider) toSnapshot(id sandbox.ID, cp *spritesdk.Checkpoint) sandbox.Snapshot {
	return sandbox.Snapshot{
		ID:        sandbox.SnapshotID(cp.ID),
		Sandbox:   id,
		Provider:  p.Name(),
		Comment:   cp.Comment,
		CreatedAt: cp.CreateTime,
	}
}

// added returns the checkpoints of after that are not in before.
func added(before, after []*spritesdk.Checkpoint) []*spritesdk.Checkpoint {
	seen := make(map[string]struct{}, len(before))
	for _, cp := range before {
		seen[cp.ID] = struct{}{}
	}
	var out []*spritesdk.Checkpoint
	for _, cp := range after {
		if _, ok := seen[cp.ID]; !ok {
			out = append(out, cp)
		}
	}
	return out
}
