package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

// SnapshotterOf returns the snapshot capability of p, or an unsupported error
// that names the missing feature.
func SnapshotterOf(p sandbox.Provider) (sandbox.Snapshotter, error) {
	s, ok := p.(sandbox.Snapshotter)
	if !ok {
		return nil, Unsupported(p, sandbox.FeatureSnapshot)
	}
	return s, nil
}

// ValidateSnapshotID rejects an ID that is not a safe single path segment.
func ValidateSnapshotID(id sandbox.SnapshotID) error {
	if sandbox.ValidSnapshotID(id) {
		return nil
	}
	return usererr.New(
		fmt.Sprintf("invalid snapshot id %q", id),
		"use the id shown by the snapshot list command",
	)
}

// ValidateSnapshotRequest checks the shared parts of a restore request: the
// snapshot ID, and a target that matches the provider's restore mode.
func ValidateSnapshotRequest(mode sandbox.RestoreMode, req sandbox.RestoreRequest) error {
	if err := ValidateSnapshotID(req.Snapshot); err != nil {
		return err
	}
	switch mode {
	case sandbox.RestoreNewSandbox:
		return validateNewSandboxTarget(req)
	case sandbox.RestoreInPlace:
		if req.Target != "" {
			return usererr.New(
				"this provider restores in place and does not take a target",
				"omit the target; the sandbox itself returns to the snapshot state",
			)
		}
		return nil
	default:
		return fmt.Errorf("unknown restore mode %q", mode)
	}
}

func validateNewSandboxTarget(req sandbox.RestoreRequest) error {
	if req.Target == "" {
		return usererr.New(
			"restore needs a target sandbox name",
			"pass a new name; the source sandbox is kept",
		)
	}
	if req.Target == req.Sandbox {
		return usererr.New(
			"the target must differ from the source sandbox",
			"pass a new name; the source sandbox is kept",
		)
	}
	return nil
}

// CollectSnapshots lists the snapshots of the named providers, in order: by
// provider, then sandbox, newest first. With a sandbox name only that sandbox
// is listed, in every provider that has it. A provider without snapshots is
// skipped. A provider that fails is returned in failures and skipped while
// another succeeds; if all fail, err joins their errors.
func CollectSnapshots(ctx context.Context, providers map[string]sandbox.Provider, names []string, name sandbox.ID) (snaps []sandbox.Snapshot, failures []error, err error) {
	supported, ok := 0, 0
	for _, pname := range names {
		s, isSnapshotter := providers[pname].(sandbox.Snapshotter)
		if !isSnapshotter {
			continue
		}
		supported++
		listed, listErr := providerSnapshots(ctx, providers[pname], s, pname, name)
		if listErr != nil {
			failures = append(failures, fmt.Errorf("%s: %w", pname, listErr))
			continue
		}
		ok++
		snaps = append(snaps, listed...)
	}
	if supported == 0 {
		return nil, nil, usererr.New(
			"no configured provider supports snapshots",
			"snapshots are available for sprites and modal; check the provider credentials",
		)
	}
	if ok == 0 {
		return nil, nil, errors.Join(failures...)
	}
	return snaps, failures, nil
}

// providerSnapshots lists the snapshots of one provider, filtered by name when
// name is not empty, and sorted.
func providerSnapshots(ctx context.Context, p sandbox.Provider, s sandbox.Snapshotter, pname string, name sandbox.ID) ([]sandbox.Snapshot, error) {
	var snaps []sandbox.Snapshot
	var err error
	if catalog, ok := s.(sandbox.SnapshotCatalog); ok {
		snaps, err = catalog.ListAllSnapshots(ctx)
	} else {
		snaps, err = snapshotsBySandbox(ctx, p, s, name)
	}
	if err != nil {
		return nil, err
	}
	out := make([]sandbox.Snapshot, 0, len(snaps))
	for _, snap := range snaps {
		if name != "" && snap.Sandbox != "" && snap.Sandbox != name {
			continue
		}
		snap.Provider = pname
		out = append(out, snap)
	}
	slices.SortStableFunc(out, func(a, b sandbox.Snapshot) int {
		if c := strings.Compare(string(a.Sandbox), string(b.Sandbox)); c != 0 {
			return c
		}
		return b.CreatedAt.Compare(a.CreatedAt)
	})
	return out, nil
}

// snapshotsBySandbox lists the sandboxes and then the snapshots of each. With
// a name it only asks for that sandbox, and only when the provider has it.
func snapshotsBySandbox(ctx context.Context, p sandbox.Provider, s sandbox.Snapshotter, name sandbox.ID) ([]sandbox.Snapshot, error) {
	infos, err := p.List(ctx)
	if err != nil {
		return nil, err
	}
	var out []sandbox.Snapshot
	for _, info := range infos {
		if name != "" && info.ID != name {
			continue
		}
		listed, err := s.ListSnapshots(ctx, info.ID)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", info.ID, err)
		}
		for _, snap := range listed {
			snap.Sandbox = info.ID
			out = append(out, snap)
		}
	}
	return out, nil
}
