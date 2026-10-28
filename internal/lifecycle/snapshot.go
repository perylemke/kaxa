package lifecycle

import (
	"fmt"

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
