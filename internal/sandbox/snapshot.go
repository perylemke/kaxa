package sandbox

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode"
)

const (
	maxSnapshotIDLen      = 128
	maxSnapshotCommentLen = 256
)

// SnapshotID identifies a snapshot of one sandbox.
type SnapshotID string

// Snapshot is a saved filesystem state of a sandbox.
type Snapshot struct {
	ID        SnapshotID
	Sandbox   ID
	Provider  string
	Comment   string
	CreatedAt time.Time
}

// RestoreMode tells how a provider brings a snapshot back.
type RestoreMode string

const (
	// RestoreInPlace returns the same sandbox to the snapshot state.
	RestoreInPlace RestoreMode = "in_place"
	// RestoreNewSandbox creates a new sandbox from the snapshot and keeps the source.
	RestoreNewSandbox RestoreMode = "new_sandbox"
)

// RestoreRequest asks a provider to restore a snapshot.
type RestoreRequest struct {
	Sandbox  ID
	Snapshot SnapshotID
	// Target names the new sandbox. Required for RestoreNewSandbox, empty for RestoreInPlace.
	Target ID
}

// Snapshotter saves and restores sandbox filesystem state.
type Snapshotter interface {
	// RestoreMode reports whether Restore overwrites the sandbox or creates a new one.
	RestoreMode() RestoreMode
	Snapshot(ctx context.Context, id ID, comment string) (Snapshot, error)
	ListSnapshots(ctx context.Context, id ID) ([]Snapshot, error)
	// Restore returns the ID of the sandbox that holds the restored state.
	Restore(ctx context.Context, req RestoreRequest) (ID, error)
	DeleteSnapshot(ctx context.Context, id ID, snap SnapshotID) error
}

// ValidSnapshotID reports whether id is a single path segment of ASCII
// letters, digits, dots, underscores, and hyphens. It keeps an ID safe to put
// in a URL path.
func ValidSnapshotID(id SnapshotID) bool {
	if id == "" || len(id) > maxSnapshotIDLen || id == "." || id == ".." {
		return false
	}
	for i := range len(id) {
		if !snapshotIDByte(id[i]) {
			return false
		}
	}
	return true
}

func snapshotIDByte(c byte) bool {
	letter := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
	return letter || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-'
}

// ValidateSnapshotComment rejects a comment that is too long or has control characters.
func ValidateSnapshotComment(comment string) error {
	if len(comment) > maxSnapshotCommentLen {
		return fmt.Errorf("comment is longer than %d bytes", maxSnapshotCommentLen)
	}
	for _, r := range comment {
		if unicode.IsControl(r) {
			return errors.New("comment has a control character")
		}
	}
	return nil
}
