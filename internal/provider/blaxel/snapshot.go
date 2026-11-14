package blaxel

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	snapReady  = "ready"
	snapFailed = "failed"
)

// RestoreMode reports that Blaxel restores a snapshot onto the same sandbox.
func (p *Provider) RestoreMode() sandbox.RestoreMode { return sandbox.RestoreInPlace }

// Snapshot saves the sandbox and waits until the snapshot is ready.
func (p *Provider) Snapshot(ctx context.Context, id sandbox.ID, comment string) (sandbox.Snapshot, error) {
	if err := sandbox.ValidateSnapshotComment(comment); err != nil {
		return sandbox.Snapshot{}, usererr.Wrap(err, err.Error(), "shorten the comment and remove control characters")
	}
	if _, _, err := p.open(ctx, id); err != nil {
		return sandbox.Snapshot{}, err
	}
	c, _, secrets, err := p.clients()
	if err != nil {
		return sandbox.Snapshot{}, err
	}
	row, err := c.CreateSnapshot(ctx, string(id))
	if err != nil {
		return sandbox.Snapshot{}, userErr(err, string(id), secrets...)
	}
	row, err = p.waitSnapshot(ctx, c, row)
	if err != nil {
		return sandbox.Snapshot{}, userErr(err, string(id), secrets...)
	}
	idx, err := p.index()
	if err != nil {
		return sandbox.Snapshot{}, err
	}
	entry := snapshotEntry{ID: row.ID, Comment: strings.TrimSpace(comment)}
	if err := idx.add(entry); err != nil {
		return sandbox.Snapshot{}, usererr.Wrap(err, "kaxa saved the snapshot but could not record its comment",
			"check that the kaxa config directory is writable")
	}
	return toSnapshot(row, entry.Comment), nil
}

// ListSnapshots returns the snapshots of one sandbox, newest first.
func (p *Provider) ListSnapshots(ctx context.Context, id sandbox.ID) ([]sandbox.Snapshot, error) {
	if err := validateName(string(id)); err != nil {
		return nil, err
	}
	c, _, secrets, err := p.clients()
	if err != nil {
		return nil, err
	}
	rows, err := c.ListSandboxSnapshots(ctx, string(id))
	if err != nil {
		return nil, userErr(err, string(id), secrets...)
	}
	return p.withComments(rows)
}

// ListAllSnapshots returns the snapshots kaxa recorded on this machine, newest first.
func (p *Provider) ListAllSnapshots(ctx context.Context) ([]sandbox.Snapshot, error) {
	c, _, secrets, err := p.clients()
	if err != nil {
		return nil, err
	}
	rows, err := c.ListWorkspaceSnapshots(ctx)
	if err != nil {
		return nil, userErr(err, "", secrets...)
	}
	idx, err := p.index()
	if err != nil {
		return nil, err
	}
	entries, err := idx.all()
	if err != nil {
		return nil, usererr.Wrap(err, "kaxa could not read the snapshot comments", "check the kaxa config directory")
	}
	out := make([]sandbox.Snapshot, 0, len(entries))
	for _, row := range rows {
		entry, ok := entries[row.ID]
		if !ok {
			continue
		}
		out = append(out, toSnapshot(row, entry.Comment))
	}
	slices.SortStableFunc(out, func(a, b sandbox.Snapshot) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return out, nil
}

// Restore returns the sandbox to a snapshot. It overwrites the sandbox and takes no target.
func (p *Provider) Restore(ctx context.Context, req sandbox.RestoreRequest) (sandbox.ID, error) {
	if req.Target != "" {
		return "", usererr.New(
			"blaxel restores in place and takes no target",
			"omit the target; the sandbox itself returns to the snapshot state",
		)
	}
	if err := validateName(string(req.Sandbox)); err != nil {
		return "", err
	}
	if !sandbox.ValidSnapshotID(req.Snapshot) {
		return "", usererr.New(
			fmt.Sprintf("invalid snapshot id %q", req.Snapshot),
			"use the id shown by the snapshot list command",
		)
	}
	c, _, secrets, err := p.clients()
	if err != nil {
		return "", err
	}
	row, err := c.GetSnapshot(ctx, string(req.Snapshot))
	if err != nil {
		return "", snapshotErr(err, req.Snapshot, secrets)
	}
	if !snapshotIsReady(row.Status) {
		return "", usererr.New(
			fmt.Sprintf("snapshot %q is %s and cannot be restored", req.Snapshot, row.Status),
			"wait for the snapshot to be ready, or take a new one",
		)
	}
	if err := c.RestoreSnapshot(ctx, string(req.Sandbox), string(req.Snapshot)); err != nil {
		return "", snapshotErr(err, req.Snapshot, secrets)
	}
	rec, err := c.Find(ctx, string(req.Sandbox))
	if err != nil {
		return "", userErr(err, string(req.Sandbox), secrets...)
	}
	if _, err := p.waitRunning(ctx, c, rec); err != nil {
		return "", userErr(err, string(req.Sandbox), secrets...)
	}
	return req.Sandbox, nil
}

// DeleteSnapshot removes a snapshot and forgets its comment.
func (p *Provider) DeleteSnapshot(ctx context.Context, id sandbox.ID, snap sandbox.SnapshotID) error {
	if err := validateName(string(id)); err != nil {
		return err
	}
	c, _, secrets, err := p.clients()
	if err != nil {
		return err
	}
	if err := c.DeleteSnapshot(ctx, string(snap)); err != nil {
		return snapshotErr(err, snap, secrets)
	}
	idx, err := p.index()
	if err != nil {
		return err
	}
	if err := idx.remove(string(snap)); err != nil {
		return usererr.Wrap(err, "the snapshot was deleted but its comment could not be forgotten",
			"check the kaxa config directory")
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

func (p *Provider) waitSnapshot(ctx context.Context, c apiClient, row snapshotRow) (snapshotRow, error) {
	timeout, interval := p.polling()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		if snapshotIsReady(row.Status) {
			return row, nil
		}
		if snapshotFailed(row.Status) {
			return snapshotRow{}, usererr.New(
				fmt.Sprintf("snapshot %q failed", row.ID),
				"take a new snapshot",
			)
		}
		if err := sleep(ctx, interval); err != nil {
			return snapshotRow{}, fmt.Errorf("blaxel: snapshot %q is %s: %w", row.ID, row.Status, err)
		}
		cur, err := c.GetSnapshot(ctx, row.ID)
		if err != nil {
			return snapshotRow{}, err
		}
		row = cur
	}
}

func (p *Provider) withComments(rows []snapshotRow) ([]sandbox.Snapshot, error) {
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
		out = append(out, toSnapshot(row, entries[row.ID].Comment))
	}
	slices.SortStableFunc(out, func(a, b sandbox.Snapshot) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return out, nil
}

func toSnapshot(row snapshotRow, comment string) sandbox.Snapshot {
	return sandbox.Snapshot{
		ID:        sandbox.SnapshotID(row.ID),
		Sandbox:   sandbox.ID(row.Sandbox),
		Provider:  ProviderName,
		Comment:   comment,
		CreatedAt: row.CreatedAt,
	}
}

func snapshotIsReady(status string) bool {
	return strings.EqualFold(status, snapReady)
}

func snapshotFailed(status string) bool {
	return strings.EqualFold(status, snapFailed)
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
