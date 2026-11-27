package vercel

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

// RestoreMode reports that Vercel restores a snapshot onto the same named
// sandbox: kaxa points the sandbox at the snapshot and resumes it.
func (p *Provider) RestoreMode() sandbox.RestoreMode { return sandbox.RestoreInPlace }

// Snapshot saves the filesystem of the sandbox. Vercel ends the session while
// it takes the snapshot; the next command resumes the sandbox. A manual
// snapshot never expires.
func (p *Provider) Snapshot(ctx context.Context, id sandbox.ID, comment string) (sandbox.Snapshot, error) {
	if err := sandbox.ValidateSnapshotComment(comment); err != nil {
		return sandbox.Snapshot{}, usererr.Wrap(err, err.Error(), "shorten the comment and remove control characters")
	}
	s, err := p.open(ctx, id)
	if err != nil {
		return sandbox.Snapshot{}, err
	}
	snap, err := s.api.CreateSnapshot(ctx, s.rec.SessionID)
	if err != nil {
		return sandbox.Snapshot{}, s.err(err)
	}
	idx, err := p.index()
	if err != nil {
		return sandbox.Snapshot{}, err
	}
	entry := snapshotEntry{ID: snap.ID, Sandbox: string(id), Comment: strings.TrimSpace(comment)}
	if err := idx.add(entry); err != nil {
		return sandbox.Snapshot{}, usererr.Wrap(err, "kaxa saved the snapshot but could not record its comment",
			"check that the kaxa config directory is writable")
	}
	return toSnapshot(snap, id, entry.Comment), nil
}

// ListSnapshots returns the manual snapshots of one sandbox, newest first.
func (p *Provider) ListSnapshots(ctx context.Context, id sandbox.ID) ([]sandbox.Snapshot, error) {
	rows, err := p.manualSnapshots(ctx, id)
	if err != nil {
		return nil, err
	}
	idx, err := p.index()
	if err != nil {
		return nil, err
	}
	entries, err := idx.all()
	if err != nil {
		return nil, usererr.Wrap(err, "kaxa could not read the snapshot comments", "check the kaxa config directory")
	}
	out := make([]sandbox.Snapshot, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSnapshot(row, id, entries[row.ID].Comment))
	}
	slices.SortStableFunc(out, func(a, b sandbox.Snapshot) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return out, nil
}

// manualSnapshots lists the usable snapshots a person took of the sandbox,
// not the automatic ones Vercel takes when a session stops.
func (p *Provider) manualSnapshots(ctx context.Context, id sandbox.ID) ([]Snapshot, error) {
	if err := validateName(string(id)); err != nil {
		return nil, err
	}
	c, secrets, err := p.clients()
	if err != nil {
		return nil, err
	}
	rows, err := c.ListSnapshots(ctx, string(id))
	if err != nil {
		return nil, userErr(err, string(id), secrets...)
	}
	return slices.DeleteFunc(rows, func(s Snapshot) bool {
		return s.Status != snapshotCreated || s.Method != snapshotManual
	}), nil
}

// Restore returns the sandbox to a snapshot. It stops the session, points the
// sandbox at the snapshot, and resumes it. It overwrites the sandbox and takes no target.
func (p *Provider) Restore(ctx context.Context, req sandbox.RestoreRequest) (sandbox.ID, error) {
	if req.Target != "" {
		return "", usererr.New(
			"vercel restores in place and takes no target",
			"omit the target; the sandbox itself returns to the snapshot state",
		)
	}
	if !sandbox.ValidSnapshotID(req.Snapshot) {
		return "", invalidSnapshotID(req.Snapshot)
	}
	if err := p.requireSnapshot(ctx, req.Sandbox, req.Snapshot); err != nil {
		return "", err
	}
	c, secrets, err := p.clients()
	if err != nil {
		return "", err
	}
	name := string(req.Sandbox)
	patch := map[string]any{"currentSnapshotId": string(req.Snapshot)}
	if _, err := p.cycle(ctx, c, name, func() error { return c.Update(ctx, name, patch) }); err != nil {
		return "", userErr(err, name, secrets...)
	}
	return req.Sandbox, nil
}

// requireSnapshot fails unless snap is a usable manual snapshot of id.
func (p *Provider) requireSnapshot(ctx context.Context, id sandbox.ID, snap sandbox.SnapshotID) error {
	rows, err := p.manualSnapshots(ctx, id)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(rows, func(s Snapshot) bool { return s.ID == string(snap) }) {
		return snapshotErr(ErrNotFound, snap, nil)
	}
	return nil
}

// DeleteSnapshot removes a snapshot of the sandbox and forgets its comment.
func (p *Provider) DeleteSnapshot(ctx context.Context, id sandbox.ID, snap sandbox.SnapshotID) error {
	if !sandbox.ValidSnapshotID(snap) {
		return invalidSnapshotID(snap)
	}
	if err := p.requireSnapshot(ctx, id, snap); err != nil {
		return err
	}
	c, secrets, err := p.clients()
	if err != nil {
		return err
	}
	if err := c.DeleteSnapshot(ctx, string(snap)); err != nil {
		return snapshotErr(err, snap, secrets)
	}
	return p.forget(func(e snapshotEntry) bool { return e.ID == string(snap) },
		"the snapshot was deleted but its comment could not be forgotten")
}

// forgetSandboxSnapshots drops the comments of a destroyed sandbox.
func (p *Provider) forgetSandboxSnapshots(name string) error {
	return p.forget(func(e snapshotEntry) bool { return e.Sandbox == name },
		"the sandbox was deleted but its snapshot comments could not be forgotten")
}

func (p *Provider) forget(drop func(snapshotEntry) bool, msg string) error {
	idx, err := p.index()
	if err != nil {
		return err
	}
	if err := idx.removeIf(drop); err != nil {
		return usererr.Wrap(err, msg, "check the kaxa config directory")
	}
	return nil
}

// SetSnapshotIndexPath sets the file that holds snapshot comments. Tests use it
// to keep the user's config directory out of reach.
func (p *Provider) SetSnapshotIndexPath(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.snapshotIndex = path
}

func (p *Provider) index() (*snapshotIndex, error) {
	p.mu.Lock()
	path := p.snapshotIndex
	p.mu.Unlock()
	if path == "" {
		var err error
		if path, err = defaultIndexPath(); err != nil {
			return nil, usererr.Wrap(err, "kaxa cannot find its config directory", "set XDG_CONFIG_HOME or HOME")
		}
	}
	return newSnapshotIndex(path), nil
}

func toSnapshot(row Snapshot, id sandbox.ID, comment string) sandbox.Snapshot {
	return sandbox.Snapshot{
		ID:        sandbox.SnapshotID(row.ID),
		Sandbox:   id,
		Provider:  ProviderName,
		Comment:   comment,
		CreatedAt: row.CreatedAt,
	}
}

func invalidSnapshotID(id sandbox.SnapshotID) error {
	return usererr.New(
		fmt.Sprintf("invalid snapshot id %q", id),
		"use the id shown by the snapshot list command",
	)
}

func snapshotErr(err error, id sandbox.SnapshotID, secrets []string) error {
	if errors.Is(err, ErrNotFound) {
		return usererr.New(
			fmt.Sprintf("snapshot %q not found", id),
			"run kaxa snapshot list to see the ids",
		)
	}
	return userErr(err, string(id), secrets...)
}
