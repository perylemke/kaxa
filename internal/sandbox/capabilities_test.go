package sandbox_test

import (
	"context"
	"io"
	"testing"

	"github.com/perylemke/sbox/internal/project"
	"github.com/perylemke/sbox/internal/sandbox"
)

type bareProvider struct{}

func (bareProvider) Name() string { return "bare" }
func (bareProvider) Create(context.Context, sandbox.Spec) (sandbox.ID, error) {
	return "", nil
}
func (bareProvider) List(context.Context) ([]sandbox.Info, error) { return nil, nil }
func (bareProvider) Inspect(context.Context, sandbox.ID) (sandbox.Detail, error) {
	return sandbox.Detail{}, nil
}
func (bareProvider) Restart(context.Context, sandbox.ID) error { return nil }
func (bareProvider) Upgrade(context.Context, sandbox.ID) error { return nil }
func (bareProvider) Exec(context.Context, sandbox.ID, sandbox.ExecOptions) error {
	return nil
}
func (bareProvider) Destroy(context.Context, sandbox.ID) error { return nil }

type fullProvider struct {
	bareProvider
	unavailable bool
}

func (fullProvider) SyncWorkspace(context.Context, sandbox.ID, io.Reader) error { return nil }
func (fullProvider) SetupProjects(context.Context, sandbox.ID, string, []project.Step) error {
	return nil
}

func (fullProvider) PublishAPI(context.Context, sandbox.ID, string, int, []string) error {
	return nil
}
func (fullProvider) SetURLAuth(context.Context, sandbox.ID, string) error { return nil }
func (fullProvider) HomeDir() string                                      { return "/home/box" }
func (fullProvider) RepoDir(repo string) (string, error)                  { return "/home/box/ws/" + repo, nil }
func (f fullProvider) Available() bool                                    { return !f.unavailable }

func TestCapabilitiesOfBareProvider(t *testing.T) {
	t.Parallel()

	got := sandbox.CapabilitiesOf(bareProvider{})
	if got != (sandbox.Capabilities{}) {
		t.Fatalf("capabilities = %+v", got)
	}
}

func TestCapabilitiesOfFullProvider(t *testing.T) {
	t.Parallel()

	got := sandbox.CapabilitiesOf(fullProvider{})
	want := sandbox.Capabilities{Sync: true, Setup: true, API: true, PublicURL: true, Layout: true}
	if got != want {
		t.Fatalf("capabilities = %+v, want %+v", got, want)
	}
}

func TestIsAvailable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		p    sandbox.Provider
		want bool
	}{
		{name: "no availability method", p: bareProvider{}, want: true},
		{name: "reports available", p: fullProvider{}, want: true},
		{name: "reports unavailable", p: fullProvider{unavailable: true}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := sandbox.IsAvailable(tt.p); got != tt.want {
				t.Fatalf("IsAvailable = %v, want %v", got, tt.want)
			}
		})
	}
}
