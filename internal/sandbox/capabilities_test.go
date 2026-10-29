package sandbox_test

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/sandbox"
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

func (bareProvider) Exec(context.Context, sandbox.ID, sandbox.ExecOptions) error {
	return nil
}
func (bareProvider) Destroy(context.Context, sandbox.ID) error { return nil }

type fullProvider struct {
	bareProvider
	unavailable bool
}

func (fullProvider) SupportsRunning()                          {}
func (fullProvider) SupportsTemplate()                         {}
func (fullProvider) Restart(context.Context, sandbox.ID) error { return nil }
func (fullProvider) Upgrade(context.Context, sandbox.ID) error { return nil }

func (fullProvider) SyncWorkspace(context.Context, sandbox.ID, io.Reader) error { return nil }
func (fullProvider) SetupProjects(context.Context, sandbox.ID, string, []project.Step) error {
	return nil
}

func (fullProvider) PublishAPI(context.Context, sandbox.ID, string, int, []string, []string) error {
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
	want := sandbox.Capabilities{Sync: true, Setup: true, API: true, PublicURL: true, Layout: true, Restart: true, Upgrade: true, Running: true, Template: true}
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

func TestCapabilitiesHas(t *testing.T) {
	t.Parallel()

	caps := sandbox.Capabilities{Restart: true, PublicURL: true, Template: true}
	tests := []struct {
		feature sandbox.Feature
		want    bool
	}{
		{sandbox.FeatureRestart, true},
		{sandbox.FeatureURLAuth, true},
		{sandbox.FeaturePublicURLs, true},
		{sandbox.FeatureTemplate, true},
		{sandbox.FeatureUpgrade, false},
		{sandbox.FeatureRunning, false},
		{sandbox.FeatureSync, false},
		{sandbox.FeatureDependencyInstall, false},
		{sandbox.FeatureAPIPublish, false},
		{sandbox.Feature("unknown"), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.feature), func(t *testing.T) {
			t.Parallel()
			if got := caps.Has(tt.feature); got != tt.want {
				t.Fatalf("Has(%q) = %v, want %v", tt.feature, got, tt.want)
			}
		})
	}
}

func TestUnsupportedError(t *testing.T) {
	t.Parallel()

	err := error(&sandbox.UnsupportedError{Provider: "modal", Feature: sandbox.FeatureRestart})
	if !errors.Is(err, sandbox.ErrUnsupported) {
		t.Fatal("UnsupportedError should wrap ErrUnsupported")
	}
	if got, want := err.Error(), "restart is not available for modal"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}
