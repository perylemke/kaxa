package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// createTemplateProvider records the Spec it was asked to create.
type createTemplateProvider struct {
	createRunningTestProvider
	spec sandbox.Spec
}

func (p *createTemplateProvider) Create(_ context.Context, spec sandbox.Spec) (sandbox.ID, error) {
	p.spec = spec
	return sandbox.ID(spec.Name), nil
}

type templateProvider struct{ *createTemplateProvider }

func (templateProvider) SupportsTemplate() {}

func runCreateTemplate(t *testing.T, p sandbox.Provider, args ...string) error {
	t.Helper()
	cmd := NewRootCmd(testRegistryWithProvider(t, p))
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs(append([]string{"--provider", spritesProviderName, "create", fixtureDemo}, args...))
	t.Setenv("KAXA_SPRITES_TOKEN", "tok")
	return cmd.Execute()
}

func TestCreateTemplateFlagReachesProvider(t *testing.T) {
	inner := &createTemplateProvider{}
	if err := runCreateTemplate(t, templateProvider{inner}, "--template", "big"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if inner.spec.Template != "big" {
		t.Fatalf("spec = %+v, want template big", inner.spec)
	}
}

func TestCreateWithoutTemplateLeavesItEmpty(t *testing.T) {
	inner := &createTemplateProvider{}
	if err := runCreateTemplate(t, templateProvider{inner}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if inner.spec.Template != "" {
		t.Fatalf("spec = %+v, want no template", inner.spec)
	}
}

func TestCreateTemplateRejectedWithoutSupport(t *testing.T) {
	inner := &createTemplateProvider{}
	err := runCreateTemplate(t, inner, "--template", "big")
	if err == nil || !strings.Contains(err.Error(), "template is not available for sprites") {
		t.Fatalf("err = %v", err)
	}
	if inner.spec.Name != "" {
		t.Fatalf("created %q before rejecting", inner.spec.Name)
	}
}
