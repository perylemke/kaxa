package lifecycle_test

import (
	"context"
	"errors"
	"testing"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/sandbox"
)

type noSnapProvider struct{}

func (noSnapProvider) Name() string { return "plain" }
func (noSnapProvider) Create(context.Context, sandbox.Spec) (sandbox.ID, error) {
	return "", nil
}
func (noSnapProvider) List(context.Context) ([]sandbox.Info, error) { return nil, nil }
func (noSnapProvider) Inspect(context.Context, sandbox.ID) (sandbox.Detail, error) {
	return sandbox.Detail{}, nil
}

func (noSnapProvider) Exec(context.Context, sandbox.ID, sandbox.ExecOptions) error {
	return nil
}
func (noSnapProvider) Destroy(context.Context, sandbox.ID) error { return nil }

func TestSnapshotterOfUnsupported(t *testing.T) {
	t.Parallel()

	_, err := lifecycle.SnapshotterOf(noSnapProvider{})
	if !errors.Is(err, sandbox.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
	ue, ok := errors.AsType[*sandbox.UnsupportedError](err)
	if !ok || ue.Feature != sandbox.FeatureSnapshot {
		t.Fatalf("unsupported error = %+v", ue)
	}
}

func TestValidateSnapshotRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mode    sandbox.RestoreMode
		req     sandbox.RestoreRequest
		wantErr bool
	}{
		{"in place ok", sandbox.RestoreInPlace, sandbox.RestoreRequest{Sandbox: "a", Snapshot: "v1"}, false},
		{"in place rejects target", sandbox.RestoreInPlace, sandbox.RestoreRequest{Sandbox: "a", Snapshot: "v1", Target: "b"}, true},
		{"new sandbox ok", sandbox.RestoreNewSandbox, sandbox.RestoreRequest{Sandbox: "a", Snapshot: "im-1", Target: "b"}, false},
		{"new sandbox needs target", sandbox.RestoreNewSandbox, sandbox.RestoreRequest{Sandbox: "a", Snapshot: "im-1"}, true},
		{"new sandbox same target", sandbox.RestoreNewSandbox, sandbox.RestoreRequest{Sandbox: "a", Snapshot: "im-1", Target: "a"}, true},
		{"bad snapshot id", sandbox.RestoreInPlace, sandbox.RestoreRequest{Sandbox: "a", Snapshot: "../x"}, true},
		{"unknown mode", sandbox.RestoreMode("x"), sandbox.RestoreRequest{Sandbox: "a", Snapshot: "v1"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := lifecycle.ValidateSnapshotRequest(tt.mode, tt.req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
