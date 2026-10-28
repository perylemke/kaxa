package modal

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

// SetSnapshotIndexPath sets the file that records the snapshots kaxa took.
// Without it the file is <user config dir>/kaxa/modal-snapshots.json.
func (p *Provider) SetSnapshotIndexPath(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.index = newSnapshotIndex(path)
}

// RestoreMode reports that Modal restores a snapshot into a new sandbox.
func (p *Provider) RestoreMode() sandbox.RestoreMode { return sandbox.RestoreNewSandbox }

// Snapshot saves the filesystem of a running sandbox as an image that Modal
// keeps for 30 days. Memory snapshots are not used: that API is experimental.
func (p *Provider) Snapshot(ctx context.Context, id sandbox.ID, comment string) (sandbox.Snapshot, error) {
	c, secrets, err := p.snapshots(id)
	if err != nil {
		return sandbox.Snapshot{}, err
	}
	if err := sandbox.ValidateSnapshotComment(comment); err != nil {
		return sandbox.Snapshot{}, usererr.Wrap(err, err.Error(), "shorten the comment and remove control characters")
	}
	idx, err := p.snapshotIndex()
	if err != nil {
		return sandbox.Snapshot{}, err
	}
	rec, err := c.Get(ctx, string(id))
	if err != nil {
		return sandbox.Snapshot{}, userErr(err, string(id), secrets...)
	}
	if rec.ExitCode != nil {
		return sandbox.Snapshot{}, userErr(ErrNotFound, string(id), secrets...)
	}
	imageID, err := c.SnapshotFilesystem(ctx, string(id))
	if err != nil {
		return sandbox.Snapshot{}, userErr(err, string(id), secrets...)
	}
	entry := snapshotEntry{
		ID: imageID, Sandbox: string(id), Comment: comment,
		CreatedAt: idx.now().UTC(), Labels: slices.Clone(rec.Labels),
	}
	if err := idx.add(entry); err != nil {
		// An image kaxa cannot list could never be deleted, so do not leave it behind.
		_ = c.DeleteImage(ctx, imageID)
		return sandbox.Snapshot{}, err
	}
	return p.toSnapshot(entry), nil
}

// ListSnapshots returns the snapshots kaxa took of a sandbox, newest first.
// It reads the local index, so it works after the sandbox has finished.
func (p *Provider) ListSnapshots(_ context.Context, id sandbox.ID) ([]sandbox.Snapshot, error) {
	if err := validateName(string(id)); err != nil {
		return nil, err
	}
	idx, err := p.snapshotIndex()
	if err != nil {
		return nil, err
	}
	entries, err := idx.forSandbox(string(id))
	if err != nil {
		return nil, err
	}
	out := make([]sandbox.Snapshot, 0, len(entries))
	for _, e := range entries {
		out = append(out, p.toSnapshot(e))
	}
	return out, nil
}

// Restore creates req.Target from a snapshot. The source sandbox is kept.
// The new sandbox has the labels of the source, so a synced sandbox keeps
// its workspace and user.
func (p *Provider) Restore(ctx context.Context, req sandbox.RestoreRequest) (sandbox.ID, error) {
	c, secrets, err := p.snapshots(req.Sandbox)
	if err != nil {
		return "", err
	}
	if err := validateName(string(req.Target)); err != nil {
		return "", err
	}
	if req.Target == req.Sandbox {
		return "", usererr.New("the target must differ from the source sandbox", "pass a new name")
	}
	entry, err := p.entry(req.Sandbox, req.Snapshot)
	if err != nil {
		return "", err
	}
	if _, err := c.CreateFromImage(ctx, string(req.Target), entry.Labels, entry.ID); err != nil {
		if errors.Is(err, ErrImageNotFound) {
			p.forget(entry.ID)
			return "", imageGoneErr(err, req.Snapshot)
		}
		return "", userErr(err, string(req.Target), secrets...)
	}
	return req.Target, nil
}

// DeleteSnapshot deletes the image of a snapshot of the sandbox and its index entry.
func (p *Provider) DeleteSnapshot(ctx context.Context, id sandbox.ID, snap sandbox.SnapshotID) error {
	c, secrets, err := p.snapshots(id)
	if err != nil {
		return err
	}
	entry, err := p.entry(id, snap)
	if err != nil {
		return err
	}
	// An image Modal already expired is as deleted as one we just removed.
	if err := c.DeleteImage(ctx, entry.ID); err != nil && !errors.Is(err, ErrImageNotFound) {
		return userErr(err, string(id), secrets...)
	}
	idx, err := p.snapshotIndex()
	if err != nil {
		return err
	}
	return idx.remove(entry.ID)
}

// snapshots validates id and returns the client that serves snapshots.
func (p *Provider) snapshots(id sandbox.ID) (fullClient, []string, error) {
	if err := validateName(string(id)); err != nil {
		return nil, nil, err
	}
	c, secrets, err := p.clientAndSecrets()
	if err != nil {
		return nil, nil, err
	}
	sc, ok := c.(fullClient)
	if !ok {
		return nil, nil, errors.New("modal: client does not support snapshots")
	}
	return sc, secrets, nil
}

func (p *Provider) snapshotIndex() (*snapshotIndex, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.index != nil {
		return p.index, nil
	}
	path, err := defaultIndexPath()
	if err != nil {
		return nil, err
	}
	p.index = newSnapshotIndex(path)
	return p.index, nil
}

// entry finds a snapshot that belongs to sandbox id.
func (p *Provider) entry(id sandbox.ID, snap sandbox.SnapshotID) (snapshotEntry, error) {
	if !sandbox.ValidSnapshotID(snap) {
		return snapshotEntry{}, usererr.New(fmt.Sprintf("invalid snapshot id %q", snap), "use an id from the snapshot list")
	}
	idx, err := p.snapshotIndex()
	if err != nil {
		return snapshotEntry{}, err
	}
	e, ok, err := idx.find(string(id), string(snap))
	if err != nil {
		return snapshotEntry{}, err
	}
	if !ok {
		return snapshotEntry{}, usererr.New(
			fmt.Sprintf("snapshot %q not found for sandbox %q", snap, id),
			"run kaxa snapshot list --provider modal to see the snapshots; Modal keeps them for 30 days and kaxa lists only those taken on this machine",
		)
	}
	return e, nil
}

// forget drops the index entry of an image Modal no longer has.
func (p *Provider) forget(imageID string) {
	if idx, err := p.snapshotIndex(); err == nil {
		_ = idx.remove(imageID)
	}
}

func imageGoneErr(err error, snap sandbox.SnapshotID) error {
	return usererr.Wrap(err, fmt.Sprintf("snapshot %q expired or was deleted", snap),
		"Modal keeps snapshot images for 30 days; take a new snapshot")
}

func (p *Provider) toSnapshot(e snapshotEntry) sandbox.Snapshot {
	return sandbox.Snapshot{
		ID:        sandbox.SnapshotID(e.ID),
		Sandbox:   sandbox.ID(e.Sandbox),
		Provider:  p.Name(),
		Comment:   e.Comment,
		CreatedAt: e.CreatedAt,
	}
}
