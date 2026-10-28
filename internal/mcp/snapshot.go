package mcp

import (
	"context"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/sandbox"
)

// snapshotter resolves the provider and its snapshot capability for a tool call.
func (s *service) snapshotter(providerName, name string) (sandbox.Provider, sandbox.Snapshotter, sandbox.ID, error) {
	p, err := s.provider(providerName)
	if err != nil {
		return nil, nil, "", err
	}
	id, err := requireName(name)
	if err != nil {
		return nil, nil, "", err
	}
	sn, err := lifecycle.SnapshotterOf(p)
	if err != nil {
		return nil, nil, "", err
	}
	return p, sn, id, nil
}

func (s *service) createSnapshot(ctx context.Context, in createSnapshotIn) (snapshotInfo, error) {
	_, sn, id, err := s.snapshotter(in.Provider, in.Name)
	if err != nil {
		return snapshotInfo{}, err
	}
	var snap sandbox.Snapshot
	err = s.progress(ctx, "creating snapshot…", func(ctx context.Context) error {
		var err error
		snap, err = sn.Snapshot(ctx, id, in.Comment)
		return err
	})
	if err != nil {
		return snapshotInfo{}, err
	}
	return toSnapshotInfo(snap), nil
}

func (s *service) listSnapshots(ctx context.Context, in nameIn) (listSnapshotsOut, error) {
	_, sn, id, err := s.snapshotter(in.Provider, in.Name)
	if err != nil {
		return listSnapshotsOut{}, err
	}
	snaps, err := sn.ListSnapshots(ctx, id)
	if err != nil {
		return listSnapshotsOut{}, err
	}
	out := listSnapshotsOut{Snapshots: make([]snapshotInfo, 0, len(snaps))}
	for _, snap := range snaps {
		out.Snapshots = append(out.Snapshots, toSnapshotInfo(snap))
	}
	return out, nil
}

func (s *service) restoreSnapshot(ctx context.Context, in restoreSnapshotIn) (snapshotActionOut, error) {
	p, sn, id, err := s.snapshotter(in.Provider, in.Name)
	if err != nil {
		return snapshotActionOut{}, err
	}
	req := sandbox.RestoreRequest{Sandbox: id, Snapshot: sandbox.SnapshotID(in.Snapshot), Target: sandbox.ID(in.Target)}
	if err := lifecycle.ValidateSnapshotRequest(sn.RestoreMode(), req); err != nil {
		return snapshotActionOut{}, err
	}
	var restored sandbox.ID
	err = s.progress(ctx, "restoring snapshot…", func(ctx context.Context) error {
		var err error
		restored, err = sn.Restore(ctx, req)
		return err
	})
	if err != nil {
		return snapshotActionOut{}, err
	}
	return snapshotActionOut{Sandbox: string(restored), Snapshot: in.Snapshot, Provider: p.Name(), Status: "restored"}, nil
}

func (s *service) deleteSnapshot(ctx context.Context, in deleteSnapshotIn) (snapshotActionOut, error) {
	p, sn, id, err := s.snapshotter(in.Provider, in.Name)
	if err != nil {
		return snapshotActionOut{}, err
	}
	snap := sandbox.SnapshotID(in.Snapshot)
	if err := lifecycle.ValidateSnapshotID(snap); err != nil {
		return snapshotActionOut{}, err
	}
	if err := sn.DeleteSnapshot(ctx, id, snap); err != nil {
		return snapshotActionOut{}, err
	}
	return snapshotActionOut{Sandbox: string(id), Snapshot: in.Snapshot, Provider: p.Name(), Status: "deleted"}, nil
}

func toSnapshotInfo(s sandbox.Snapshot) snapshotInfo {
	return snapshotInfo{
		ID: string(s.ID), Sandbox: string(s.Sandbox), Provider: s.Provider,
		Comment: s.Comment, CreatedAt: formatTime(s.CreatedAt),
	}
}
