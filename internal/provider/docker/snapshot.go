package docker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

// RestoreMode reports that Docker restores a snapshot into a new sandbox.
func (p *Provider) RestoreMode() sandbox.RestoreMode { return sandbox.RestoreNewSandbox }

// Snapshot saves the disk of a sandbox and waits until the snapshot is ready.
// A stopped sandbox is started first, because Docker only snapshots a running
// one. Memory is not captured.
func (p *Provider) Snapshot(ctx context.Context, id sandbox.ID, comment string) (sandbox.Snapshot, error) {
	if err := sandbox.ValidateSnapshotComment(comment); err != nil {
		return sandbox.Snapshot{}, usererr.Wrap(err, err.Error(), "shorten the comment and remove control characters")
	}
	api, rec, err := p.find(ctx, id)
	if err != nil {
		return sandbox.Snapshot{}, err
	}
	// A fresh read gives the current status and etag.
	rec, err = api.Get(ctx, rec.Resource)
	if err != nil {
		return sandbox.Snapshot{}, p.fail(api, err, string(id))
	}
	if rec, err = p.ensureRunning(ctx, api, rec); err != nil {
		return sandbox.Snapshot{}, p.fail(api, err, string(id))
	}
	snap, err := api.CreateSnapshot(ctx, rec, snapshotDisplayName(string(id), time.Now()), encodeSnapshotMeta(rec.Labels, comment))
	if err != nil {
		return sandbox.Snapshot{}, p.snapshotFail(api, err, string(id))
	}
	if snap, err = p.waitSnapshot(ctx, api, snap); err != nil {
		return sandbox.Snapshot{}, p.snapshotFail(api, err, string(id))
	}
	return toSnapshot(snap), nil
}

// ListSnapshots returns the snapshots kaxa took of a sandbox, newest first.
// It works after the sandbox was destroyed, because Docker keeps the snapshot.
func (p *Provider) ListSnapshots(ctx context.Context, id sandbox.ID) ([]sandbox.Snapshot, error) {
	if err := validateName(string(id)); err != nil {
		return nil, err
	}
	all, err := p.ListAllSnapshots(ctx)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(all, func(s sandbox.Snapshot) bool { return s.Sandbox != id }), nil
}

// ListAllSnapshots returns every snapshot kaxa took with this account, newest
// first, including those of sandboxes that were destroyed. Snapshots that kaxa
// did not take are left out.
func (p *Provider) ListAllSnapshots(ctx context.Context) ([]sandbox.Snapshot, error) {
	api, err := p.ready()
	if err != nil {
		return nil, err
	}
	listed, err := api.ListSnapshots(ctx)
	if err != nil {
		return nil, p.snapshotFail(api, err, "")
	}
	out := make([]sandbox.Snapshot, 0, len(listed))
	for _, s := range listed {
		if s.Sandbox() == "" || s.Status == snapshotDeleted {
			continue
		}
		// The list view has no comment. A snapshot whose detail cannot be read
		// is still listed, only without it.
		if full, err := api.GetSnapshot(ctx, s.Resource); err == nil {
			s.Description = full.Description
		}
		out = append(out, toSnapshot(s))
	}
	slices.SortStableFunc(out, func(a, b sandbox.Snapshot) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return out, nil
}

// Restore creates req.Target from a snapshot. The source sandbox and the
// snapshot are kept.
func (p *Provider) Restore(ctx context.Context, req sandbox.RestoreRequest) (sandbox.ID, error) {
	if err := validateName(string(req.Target)); err != nil {
		return "", err
	}
	if req.Target == req.Sandbox {
		return "", usererr.New("the target must differ from the source sandbox", "pass a new name")
	}
	api, snap, err := p.ownedSnapshot(ctx, req.Sandbox, req.Snapshot)
	if err != nil {
		return "", err
	}
	if snap.Status != snapshotReady {
		return "", usererr.New(
			fmt.Sprintf("snapshot %q is %s and cannot be restored", req.Snapshot, snap.Status),
			"wait for the snapshot to be ready, or take a new one")
	}
	if _, err := api.Find(ctx, string(req.Target)); err == nil {
		return "", p.fail(api, ErrAlreadyExists, string(req.Target))
	} else if !errors.Is(err, ErrNotFound) {
		return "", p.fail(api, err, string(req.Target))
	}
	rec, err := api.RestoreSnapshot(ctx, snap, string(req.Target))
	if err != nil {
		return "", p.snapshotFail(api, err, string(req.Target))
	}
	// The answer to a restore may be a partial view of the sandbox, so it is
	// read again.
	cur, err := api.Get(ctx, rec.Resource)
	if err != nil {
		return "", errors.Join(p.fail(api, err, string(req.Target)), p.removeBest(ctx, api, rec))
	}
	if err := p.adoptRestored(cur, snap); err != nil {
		return "", errors.Join(err, p.removeBest(ctx, api, cur))
	}
	if _, err := p.ensureRunning(ctx, api, cur); err != nil {
		return "", errors.Join(p.fail(api, err, string(req.Target)), p.removeBest(ctx, api, cur), p.forgetRestored(cur.Resource))
	}
	return req.Target, nil
}

// adoptRestored makes kaxa recognize a restored sandbox as its own. Docker
// gives a restored sandbox no labels and cannot add them later, so kaxa
// records the sandbox and the labels of the snapshot's source in a local
// file. A sandbox that already has the labels needs no record.
func (p *Provider) adoptRestored(cur Record, snap SnapshotRecord) error {
	if cur.Managed() {
		return nil
	}
	idx, err := p.restoredIdx()
	if err != nil {
		return restoredIndexErr(err)
	}
	if err := idx.add(restoredEntry{Resource: cur.Resource, Name: cur.Name, Labels: snap.meta().Labels}); err != nil {
		return restoredIndexErr(err)
	}
	return nil
}

func restoredIndexErr(err error) error {
	return usererr.Wrap(err, "kaxa could not record the restored sandbox, so it removed it",
		"check that the kaxa config directory is writable; the snapshot was kept")
}

// forgetRestored drops a sandbox from the index of restored sandboxes.
func (p *Provider) forgetRestored(resource string) error {
	idx, err := p.restoredIdx()
	if err != nil {
		return err
	}
	return idx.remove(resource)
}

// DeleteSnapshot deletes a snapshot of the sandbox. A snapshot that is already
// gone counts as deleted.
func (p *Provider) DeleteSnapshot(ctx context.Context, id sandbox.ID, snap sandbox.SnapshotID) error {
	api, rec, err := p.ownedSnapshot(ctx, id, snap)
	if err != nil {
		return err
	}
	if err := api.DeleteSnapshot(ctx, rec); err != nil && !errors.Is(err, ErrSnapshotNotFound) {
		return p.deleteSnapshotFail(api, err, id, snap)
	}
	return nil
}

// deleteSnapshotFail explains a refused delete. Docker answers 409 while a
// sandbox restored from the snapshot is still running, or while the snapshot is
// still being created, and any other failure is the usual one.
func (p *Provider) deleteSnapshotFail(api apiClient, err error, id sandbox.ID, snap sandbox.SnapshotID) error {
	apiErr, ok := errors.AsType[*APIError](err)
	if !ok || apiErr.Status != http.StatusConflict {
		return p.snapshotFail(api, err, string(id))
	}
	reason := redact(apiErr.Message, api.secrets())
	if reason == "" {
		reason = apiErr.Code
	}
	return usererr.Wrap(apiErr, fmt.Sprintf("Docker cannot delete snapshot %q now: %s", snap, reason),
		"if a sandbox restored from it is still running, destroy it first (kaxa list shows them); if the snapshot is still being created, wait and try again")
}

// ownedSnapshot reads a snapshot and checks that kaxa took it of that sandbox.
// Any other snapshot is reported as not found: kaxa never touches the user's
// own snapshots.
func (p *Provider) ownedSnapshot(ctx context.Context, id sandbox.ID, snap sandbox.SnapshotID) (apiClient, SnapshotRecord, error) {
	if err := validateName(string(id)); err != nil {
		return nil, SnapshotRecord{}, err
	}
	if !sandbox.ValidSnapshotID(snap) {
		return nil, SnapshotRecord{}, usererr.New(fmt.Sprintf("invalid snapshot id %q", snap), "use an id from the snapshot list")
	}
	api, err := p.ready()
	if err != nil {
		return nil, SnapshotRecord{}, err
	}
	rec, err := api.GetSnapshot(ctx, "snapshots/"+string(snap))
	if err != nil && !errors.Is(err, ErrSnapshotNotFound) {
		return nil, SnapshotRecord{}, p.snapshotFail(api, err, string(id))
	}
	if err != nil || rec.Sandbox() != string(id) || rec.Status == snapshotDeleted {
		return nil, SnapshotRecord{}, usererr.New(
			fmt.Sprintf("snapshot %q not found for sandbox %q", snap, id),
			fmt.Sprintf("run kaxa snapshot list %s --provider docker to see the snapshots", id))
	}
	return api, rec, nil
}

// waitSnapshot polls a snapshot until it is ready. A snapshot that failed is
// deleted, because it holds no usable state.
func (p *Provider) waitSnapshot(ctx context.Context, api apiClient, snap SnapshotRecord) (SnapshotRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, p.wait)
	defer cancel()
	for {
		switch snap.Status {
		case snapshotReady:
			return snap, nil
		case snapshotFailed, snapshotDeleted:
			return SnapshotRecord{}, errors.Join(snapshotFailedErr(snap), p.removeSnapshotBest(ctx, api, snap))
		}
		select {
		case <-ctx.Done():
			return SnapshotRecord{}, fmt.Errorf("docker: snapshot %q is %s: %w", snap.ID, snap.Status, ctx.Err())
		case <-time.After(p.poll):
		}
		cur, err := api.GetSnapshot(ctx, snap.Resource)
		if err != nil {
			return SnapshotRecord{}, err
		}
		snap = cur
	}
}

func snapshotFailedErr(s SnapshotRecord) error {
	if s.Failure == "" {
		return fmt.Errorf("docker: snapshot %q failed", s.ID)
	}
	return fmt.Errorf("docker: snapshot %q failed: %s", s.ID, s.Failure)
}

// removeSnapshotBest deletes a snapshot after a failure and reports why it could not.
func (p *Provider) removeSnapshotBest(ctx context.Context, api apiClient, snap SnapshotRecord) error {
	rmCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	if cur, err := api.GetSnapshot(rmCtx, snap.Resource); err == nil {
		snap = cur
	}
	if err := api.DeleteSnapshot(rmCtx, snap); err != nil && !errors.Is(err, ErrSnapshotNotFound) {
		return fmt.Errorf("remove snapshot: %w", p.fail(api, err, snap.ID))
	}
	return nil
}

// snapshotFail turns an error of the snapshot API into a user-facing error,
// without credentials.
func (p *Provider) snapshotFail(api apiClient, err error, name string) error {
	if errors.Is(err, ErrSnapshotExists) {
		return usererr.Wrap(err, "a snapshot with that name already exists", "wait a second and try again")
	}
	if apiErr, ok := errors.AsType[*APIError](err); ok && apiErr.is(codePermissionDenied, http.StatusForbidden) {
		return usererr.Wrap(apiErr, "Docker denied access to snapshots",
			"the personal access token needs permission to read and write snapshots (snapshotsRead, snapshotsWrite); check the subscription and the scopes of the token")
	}
	return p.fail(api, err, name)
}

func toSnapshot(s SnapshotRecord) sandbox.Snapshot {
	return sandbox.Snapshot{
		ID:        sandbox.SnapshotID(s.ID),
		Sandbox:   sandbox.ID(s.Sandbox()),
		Provider:  ProviderName,
		Comment:   strings.TrimSpace(s.meta().Comment),
		CreatedAt: s.CreatedAt,
	}
}
