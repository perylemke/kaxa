package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/perylemke/sbox/internal/registry"
	"github.com/perylemke/sbox/internal/sandbox"
)

type createRunningTestProvider struct {
	url string
}

func (p *createRunningTestProvider) Name() string { return spritesProviderName }

func (p *createRunningTestProvider) Configure(string) error { return nil }

func (p *createRunningTestProvider) Create(_ context.Context, spec sandbox.Spec) (sandbox.ID, error) {
	return sandbox.ID(spec.Name), nil
}

func (p *createRunningTestProvider) List(context.Context) ([]sandbox.Info, error) {
	return nil, nil
}

func (p *createRunningTestProvider) Inspect(_ context.Context, id sandbox.ID) (sandbox.Detail, error) {
	return sandbox.Detail{ID: id, URL: p.url}, nil
}

func (p *createRunningTestProvider) Restart(context.Context, sandbox.ID) error { return nil }

func (p *createRunningTestProvider) Upgrade(context.Context, sandbox.ID) error { return nil }

func (p *createRunningTestProvider) Exec(context.Context, sandbox.ID, sandbox.ExecOptions) error {
	return nil
}

func (p *createRunningTestProvider) Destroy(context.Context, sandbox.ID) error { return nil }

func testRegistryWithProvider(t *testing.T, p sandbox.Provider) *registry.Registry {
	t.Helper()
	reg := registry.New()
	if err := reg.Register(p); err != nil {
		t.Fatalf("register: %v", err)
	}
	return reg
}

func TestCreateRunningOpensBrowserURL(t *testing.T) {
	var opened string
	prev := openBrowserFn
	openBrowserFn = func(url string) error {
		opened = url
		return nil
	}
	t.Cleanup(func() { openBrowserFn = prev })

	const url = "https://demo-bpcxj.sprites.app"
	p := &createRunningTestProvider{url: url}
	reg := testRegistryWithProvider(t, p)
	cmd := NewRootCmd(reg)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"--provider", spritesProviderName, "create", fixtureDemo, "--running"})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if opened != url {
		t.Fatalf("opened = %q, want %q", opened, url)
	}
	if !strings.Contains(out.String(), url) {
		t.Fatalf("output = %q", out.String())
	}
}

func TestCreateWithoutRunningDoesNotOpenBrowser(t *testing.T) {
	var opened string
	prev := openBrowserFn
	openBrowserFn = func(url string) error {
		opened = url
		return nil
	}
	t.Cleanup(func() { openBrowserFn = prev })

	p := &createRunningTestProvider{url: "https://demo.sprites.app"}
	reg := testRegistryWithProvider(t, p)
	cmd := NewRootCmd(reg)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"--provider", spritesProviderName, "create", fixtureDemo})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if opened != "" {
		t.Fatalf("opened = %q", opened)
	}
}
