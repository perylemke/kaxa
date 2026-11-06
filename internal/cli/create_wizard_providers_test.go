package cli_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/cli"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

// wizardModalUnconfigured is registered as "modal" but never configures, so ListTargets skips it.
type wizardModalUnconfigured struct {
	fakeProvider
}

func (w *wizardModalUnconfigured) Name() string { return testModalName }

func (*wizardModalUnconfigured) ConfigureCredentials(_, _ string) error {
	return sandbox.ErrNotConfigured
}

// wizardModalConfigured registers as modal and configures like the real provider.
type wizardModalConfigured struct {
	fakeProvider
}

func (*wizardModalConfigured) Name() string { return testModalName }

func (*wizardModalConfigured) ConfigureCredentials(id, secret string) error {
	if id == "" || secret == "" {
		return sandbox.ErrNotConfigured
	}
	return nil
}

func TestCreateWizardListsOnlyConfiguredProviders(t *testing.T) {
	sp := &fakeProvider{name: testSpritesName}
	md := &wizardModalUnconfigured{fakeProvider{name: testModalName}}
	reg := wizardRegistry(t, sp, md)

	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs([]string{testCmdCreate})
	t.Setenv("KAXA_SPRITES_TOKEN", "tok")
	// One configured provider: confirm sprites, then the name, then sprites options.
	cmd.SetIn(strings.NewReader(strings.Join([]string{
		"1", testBoxName,
		"n", "n", "", "n", "n",
	}, "\n")))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	combined := errOut.String() + out.String()
	if !strings.Contains(combined, "Provider") || !strings.Contains(combined, "1. sprites") || !strings.Contains(combined, "Name") {
		t.Fatalf("wizard should show sprites selected and the Name field: %q", combined)
	}
}

func TestCreateWizardWithProviderFlagListsOnlyThatProvider(t *testing.T) {
	sp := &fakeProvider{name: testSpritesName}
	md := &wizardModalConfigured{}
	reg := wizardRegistry(t, sp, md)

	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs([]string{testProviderFlag, testModalName, testCmdCreate})
	t.Setenv("KAXA_SPRITES_TOKEN", "tok")
	t.Setenv("KAXA_MODAL_TOKEN_ID", "ak")
	t.Setenv("KAXA_MODAL_TOKEN_SECRET", "sk")
	// --provider modal: only modal is listed; confirm it, then the name, then modal options.
	cmd.SetIn(strings.NewReader(strings.Join([]string{
		"1", testBoxName,
		"n", "1", "1",
	}, "\n")))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	combined := errOut.String() + out.String()
	if !strings.Contains(combined, "Provider") || !strings.Contains(combined, "1. modal") || !strings.Contains(combined, "Name") || strings.Contains(combined, "sprites") {
		t.Fatalf("wizard should show only modal selected and the Name field: %q", combined)
	}
	if md.lastCreate.Name != testBoxName {
		t.Fatalf("modal create = %+v", md.lastCreate)
	}
}

func TestCreateWizardListsAllConfiguredProvidersWhenNoProviderFlag(t *testing.T) {
	sp := &fakeProvider{name: testSpritesName}
	md := &wizardModalConfigured{}
	reg := wizardRegistry(t, sp, md)

	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs([]string{testCmdCreate})
	t.Setenv("KAXA_SPRITES_TOKEN", "tok")
	t.Setenv("KAXA_MODAL_TOKEN_ID", "ak")
	t.Setenv("KAXA_MODAL_TOKEN_SECRET", "sk")
	// Registry sorts names: modal=1, sprites=2; pick modal, name, then modal options.
	cmd.SetIn(strings.NewReader(strings.Join([]string{
		"1", testBoxName,
		"n", "1", "1",
	}, "\n")))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	combined := errOut.String() + out.String()
	// Registry orders names; modal precedes sprites.
	if !strings.Contains(combined, "1. modal") || !strings.Contains(combined, "2. sprites") {
		t.Fatalf("want both providers listed: %q", combined)
	}
}

func TestCreateWizardProviderFlagUnconfiguredFailsBeforeForm(t *testing.T) {
	sp := &fakeProvider{name: testSpritesName}
	md := &wizardModalUnconfigured{fakeProvider{name: testModalName}}
	reg := wizardRegistry(t, sp, md)

	cmd := cli.NewRootCmd(reg)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{testProviderFlag, testModalName, testCmdCreate})
	t.Setenv("KAXA_SPRITES_TOKEN", "tok")

	err := cmd.Execute()
	if err == nil || !errors.Is(err, sandbox.ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

func TestCreateWizardFailsWhenNoProviderConfigured(t *testing.T) {
	sp := &fakeProvider{name: testSpritesName}
	// Sprites token unset: ResolveProvider fails Configure with ErrMissingToken wrapping ErrNotConfigured.
	reg := wizardRegistry(t, sp)

	cmd := cli.NewRootCmd(reg)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetIn(strings.NewReader("\n"))
	cmd.SetArgs([]string{testCmdCreate})

	err := cmd.Execute()
	ue, ok := usererr.As(err)
	if !ok || ue.Message != "no provider is configured" {
		t.Fatalf("err = %v", err)
	}
}

func wizardRegistry(t *testing.T, ps ...sandbox.Provider) *registry.Registry {
	t.Helper()
	reg := registry.New()
	for _, p := range ps {
		if err := reg.Register(p); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	return reg
}
