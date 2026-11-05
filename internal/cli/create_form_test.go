package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

const formModalProvider = "modal"

type formModalFake struct{ formBareProvider }

func (formModalFake) PrepareWorkspace(context.Context, sandbox.ID) error { return nil }
func (formModalFake) SyncWorkspace(context.Context, sandbox.ID, io.Reader) error {
	return nil
}

type formSpritesFake struct{ formBareProvider }

func (formSpritesFake) SupportsRunning()                                   {}
func (formSpritesFake) PrepareWorkspace(context.Context, sandbox.ID) error { return nil }
func (formSpritesFake) SyncWorkspace(context.Context, sandbox.ID, io.Reader) error {
	return nil
}

func (formSpritesFake) PublishAPI(context.Context, sandbox.ID, string, int, []string, []string) error {
	return nil
}
func (formSpritesFake) SetURLAuth(context.Context, sandbox.ID, string) error { return nil }

type formE2BFake struct{ formBareProvider }

func (formE2BFake) SupportsTemplate()                                  {}
func (formE2BFake) PrepareWorkspace(context.Context, sandbox.ID) error { return nil }
func (formE2BFake) SyncWorkspace(context.Context, sandbox.ID, io.Reader) error {
	return nil
}

func (formE2BFake) PublishAPI(context.Context, sandbox.ID, string, int, []string, []string) error {
	return nil
}

func formWizardLookup(name string) (sandbox.Provider, error) {
	switch name {
	case formModalProvider:
		return formModalFake{formBareProvider{name: formModalProvider}}, nil
	case spritesProviderName:
		return formSpritesFake{formBareProvider{name: spritesProviderName}}, nil
	case e2bProviderName:
		return formE2BFake{formBareProvider{name: e2bProviderName}}, nil
	default:
		return nil, errors.New("unknown provider")
	}
}

func formProviders() []string {
	return []string{formModalProvider, spritesProviderName, e2bProviderName}
}

func TestCreateFormModalAccessible(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{"1", fixtureDemo, "n", ""}, "\n"))
	out := new(bytes.Buffer)

	got, err := collectCreateForm(in, out, formProviders(), spritesProviderName, formWizardLookup, createFlags{}, false)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if got.Name != fixtureDemo || got.Provider != formModalProvider || got.Running || got.Sync || got.API || got.Public {
		t.Fatalf("result = %+v", got)
	}
	view := out.String()
	for _, title := range []string{"Name", "Provider", "Sync"} {
		if !strings.Contains(view, title) {
			t.Fatalf("prompts missing %q: %q", title, view)
		}
	}
	for _, title := range []string{"Running", "Public", "API", "Template"} {
		if strings.Contains(view, title) {
			t.Fatalf("prompts should not contain %q: %q", title, view)
		}
	}
}

func TestCreateFormSpritesAccessibleDefaults(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{"", fixturePery, "", "", "", "", ""}, "\n"))
	out := new(bytes.Buffer)

	got, err := collectCreateForm(in, out, formProviders(), spritesProviderName, formWizardLookup, createFlags{Running: true, Sync: true}, false)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if got.Name != fixturePery || got.Provider != spritesProviderName || !got.Running || !got.Sync || got.API || got.Public {
		t.Fatalf("result = %+v", got)
	}
	view := out.String()
	for _, title := range []string{"Running", "Sync", "Public"} {
		if !strings.Contains(view, title) {
			t.Fatalf("prompts missing %q: %q", title, view)
		}
	}
}

func TestCreateFormE2BAccessible(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	in := strings.NewReader(strings.Join([]string{
		"3", fixturePery, "y", "my-template", "y", "", "n",
		".venv/bin/uvicorn,app.main:app", "9000", "DATABASE_URL",
	}, "\n"))
	out := new(bytes.Buffer)

	got, err := collectCreateForm(in, out, formProviders(), spritesProviderName, formWizardLookup, createFlags{}, false)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if got.Provider != e2bProviderName || got.Running || !got.Sync || !got.API || got.Public || got.Template != "my-template" {
		t.Fatalf("result = %+v", got)
	}
	if got.APIPort != 9000 || len(got.APICmd) != 2 || got.APICmd[0] != ".venv/bin/uvicorn" {
		t.Fatalf("api = port %d cmd %#v", got.APIPort, got.APICmd)
	}
	assertFormPrompts(t, out.String(),
		[]string{"Template", "Sync", "API", "Directory", "Skip install", "API command", "API port", "API env"},
		[]string{"Running", "Public"},
	)
}

func assertFormPrompts(t *testing.T, view string, want, omit []string) {
	t.Helper()
	for _, title := range want {
		if !strings.Contains(view, title) {
			t.Fatalf("prompts missing %q: %q", title, view)
		}
	}
	for _, title := range omit {
		if strings.Contains(view, title) {
			t.Fatalf("prompts should not contain %q: %q", title, view)
		}
	}
}

func TestCreateFormSpritesSyncPage(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		"2", fixturePery, "n", "y", "n", "n", "", "y",
	}, "\n"))
	out := new(bytes.Buffer)

	got, err := collectCreateForm(in, out, formProviders(), spritesProviderName, formWizardLookup, createFlags{}, false)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if !got.Sync || !got.NoInstall || got.DirSet {
		t.Fatalf("sync = %+v", got)
	}
	if !strings.Contains(out.String(), "Directory") || !strings.Contains(out.String(), "Skip install") {
		t.Fatalf("prompts = %q", out.String())
	}
}

func TestCreateFormRunningFlagWithModalFails(t *testing.T) {
	p := formModalFake{formBareProvider{name: formModalProvider}}
	err := checkCreateFormFlags(p, createFlags{Running: true})
	if !errors.Is(err, sandbox.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}

func TestCreateFormPublicRunningWithoutAPIFails(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		"2", fixturePery, "y", "n", "n", "y",
	}, "\n"))
	out := new(bytes.Buffer)

	_, err := collectCreateForm(in, out, formProviders(), spritesProviderName, formWizardLookup, createFlags{}, false)
	if err == nil || !strings.Contains(err.Error(), "tmux terminal") {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateFormHiddenRunningNotInResult(t *testing.T) {
	var result createFormResult
	result.Provider = formModalProvider
	result.Running = true
	applyCreateFormCapabilities(&result, sandbox.CapabilitiesOf(formModalFake{formBareProvider{name: formModalProvider}}))
	if result.Running {
		t.Fatalf("running = true after modal capabilities")
	}
}

func TestApplyCreateFormCapabilitiesClearsOnProviderChange(t *testing.T) {
	var result createFormResult
	result.Running = true
	result.Public = true
	result.API = true
	result.Template = "img"
	result.Sync = true
	result.Dir = "/tmp/x"
	result.DirSet = true
	result.NoInstall = true

	applyCreateFormCapabilities(&result, sandbox.CapabilitiesOf(formModalFake{formBareProvider{name: formModalProvider}}))
	if result.Running || result.Public || result.API || result.Template != "" {
		t.Fatalf("modal caps left provider-only fields: %+v", result)
	}
	if !result.Sync {
		t.Fatal("sync should stay enabled on modal")
	}
	result.Sync = false
	applyCreateFormCapabilities(&result, sandbox.CapabilitiesOf(formModalFake{formBareProvider{name: formModalProvider}}))
	if result.DirSet || result.NoInstall || result.Dir != "" {
		t.Fatalf("sync off should clear dir: %+v", result)
	}
}
