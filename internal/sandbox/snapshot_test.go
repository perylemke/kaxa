package sandbox_test

import (
	"context"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

type snapProvider struct {
	bareProvider
	mode sandbox.RestoreMode
}

func (s snapProvider) RestoreMode() sandbox.RestoreMode { return s.mode }
func (snapProvider) Snapshot(context.Context, sandbox.ID, string) (sandbox.Snapshot, error) {
	return sandbox.Snapshot{}, nil
}

func (snapProvider) ListSnapshots(context.Context, sandbox.ID) ([]sandbox.Snapshot, error) {
	return nil, nil
}

func (snapProvider) Restore(context.Context, sandbox.RestoreRequest) (sandbox.ID, error) {
	return "", nil
}

func (snapProvider) DeleteSnapshot(context.Context, sandbox.ID, sandbox.SnapshotID) error { return nil }

func TestCapabilitiesOfSnapshotter(t *testing.T) {
	t.Parallel()

	got := sandbox.CapabilitiesOf(snapProvider{mode: sandbox.RestoreNewSandbox})
	want := sandbox.Capabilities{Snapshot: true, SnapshotRestore: string(sandbox.RestoreNewSandbox)}
	if got != want {
		t.Fatalf("capabilities = %+v, want %+v", got, want)
	}
	if !got.Has(sandbox.FeatureSnapshot) {
		t.Fatal("Has(snapshot) = false")
	}
	if sandbox.CapabilitiesOf(bareProvider{}).Has(sandbox.FeatureSnapshot) {
		t.Fatal("bare provider should not have snapshot")
	}
}

func TestValidSnapshotID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		id   string
		want bool
	}{
		{"v7", true},
		{"im-AbC123", true},
		{"a.b_c-d", true},
		{"", false},
		{".", false},
		{"..", false},
		{"a/b", false},
		{"a b", false},
		{"a?x=1", false},
		{"é", false},
		{strings.Repeat("a", 129), false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			if got := sandbox.ValidSnapshotID(sandbox.SnapshotID(tt.id)); got != tt.want {
				t.Fatalf("ValidSnapshotID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestValidateSnapshotComment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		comment string
		wantErr bool
	}{
		{"empty", "", false},
		{"plain", "before migration", false},
		{"unicode", "antes da migração", false},
		{"newline", "a\nb", true},
		{"escape", "a\x1bb", true},
		{"too long", strings.Repeat("a", 257), true},
		{"max", strings.Repeat("a", 256), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := sandbox.ValidateSnapshotComment(tt.comment)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
