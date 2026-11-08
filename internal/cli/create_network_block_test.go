package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

type networkBlockProvider struct {
	createRunningTestProvider
	spec sandbox.Spec
}

func (networkBlockProvider) Name() string { return "daytona" }

func (networkBlockProvider) SupportsNetworkBlock() {}

func (p *networkBlockProvider) Create(_ context.Context, spec sandbox.Spec) (sandbox.ID, error) {
	p.spec = spec
	return sandbox.ID(spec.Name), nil
}

func TestCreateNetworkBlockFlagReachesDaytona(t *testing.T) {
	p := &networkBlockProvider{}
	cmd := NewRootCmd(testRegistryWithProvider(t, p))
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{flagProvider, "daytona", commandCreate, fixtureDemo, "--block-all-network-access"})
	t.Setenv("KAXA_DAYTONA_API_KEY", "key")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if p.spec.Name != fixtureDemo || !p.spec.NetworkBlockAll {
		t.Fatalf("spec = %+v", p.spec)
	}
}

func TestCreateNetworkBlockRejectedOnSprites(t *testing.T) {
	inner := &createTemplateProvider{}
	err := runCreateTemplate(t, inner, "--block-all-network-access")
	if err == nil || !strings.Contains(err.Error(), "network block is not available for sprites") {
		t.Fatalf("err = %v", err)
	}
	if inner.spec.Name != "" {
		t.Fatalf("created %q before rejecting", inner.spec.Name)
	}
}
