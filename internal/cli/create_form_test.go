package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"

	"charm.land/huh/v2"
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

func TestCollectCreateFormNoProviders(t *testing.T) {
	t.Parallel()

	_, err := collectCreateForm(strings.NewReader(""), new(bytes.Buffer), nil, "", formWizardLookup, createFlags{}, false)
	if err == nil || !strings.Contains(err.Error(), "no sandbox providers registered") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateCreateFlagsPreFormRejectsOrphanFlags(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		flags   createFlags
		portSet bool
		want    string
	}{
		{name: "api-cmd", flags: createFlags{APICmd: "serve"}, want: "--api-cmd requires --api"},
		{name: "api-port", flags: createFlags{APIPort: 8000}, portSet: true, want: "--api-port requires --api"},
		{name: "api-env", flags: createFlags{APIEnv: []string{"FOO=1"}}, want: "--api-env requires --api"},
		{name: "dir", flags: createFlags{Dir: "/tmp", DirSet: true}, want: "--dir requires --sync"},
		{name: "no-install", flags: createFlags{NoInstall: true}, want: "--no-install requires --sync"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateCreateFlagsPreForm(tc.flags, tc.portSet)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCreateFormPreFormFailsBeforeWizard(t *testing.T) {
	t.Parallel()

	_, err := collectCreateForm(
		strings.NewReader(""),
		new(bytes.Buffer),
		formProviders(),
		spritesProviderName,
		formWizardLookup,
		createFlags{APICmd: "x"},
		false,
	)
	if err == nil || !strings.Contains(err.Error(), "--api-cmd requires --api") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateCreateNameEmpty(t *testing.T) {
	t.Parallel()
	if err := validateCreateName("   "); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateCreateRunningPublicWithoutAPI(t *testing.T) {
	t.Parallel()
	r := &createFormResult{Public: true, API: false}
	if err := validateCreateRunning(true, r); err == nil || !strings.Contains(err.Error(), "tmux terminal") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateCreatePublicRunningWithoutAPI(t *testing.T) {
	t.Parallel()
	r := &createFormResult{Running: true, API: false}
	if err := validateCreatePublic(true, r); err == nil || !strings.Contains(err.Error(), "tmux terminal") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateCreateSyncAnswerFalseClearsSyncFields(t *testing.T) {
	t.Parallel()
	result := createFormResult{Dir: "/tmp/x", DirSet: true, NoInstall: true}
	if err := validateCreateSyncAnswer(false, &result); err != nil {
		t.Fatal(err)
	}
	if result.Dir != "" || result.DirSet || result.NoInstall {
		t.Fatalf("result = %+v", result)
	}
}

func TestValidateCreateAPIAnswerFalseResetsRaw(t *testing.T) {
	t.Parallel()
	raw := &createFormAPIRaw{command: "a,b", port: "9000", env: "X"}
	if err := validateCreateAPIAnswer(false, raw); err != nil {
		t.Fatal(err)
	}
	if raw.command != "" || raw.env != "" || raw.port != strconv.Itoa(defaultAPIPort) {
		t.Fatalf("raw = %+v", raw)
	}
}

func TestValidateCreateSyncDirInvalidPath(t *testing.T) {
	t.Parallel()
	err := validateCreateSyncDir("/no-such-directory-kaxa-test-12345")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateCreateAPIEnvInputInvalidKey(t *testing.T) {
	t.Parallel()
	err := validateCreateAPIEnvInput("123BAD")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFinalizeCreateFormResultPublicRunningWithoutAPI(t *testing.T) {
	t.Parallel()
	lookup := func(string) (sandbox.Provider, error) {
		return formSpritesFake{formBareProvider{name: spritesProviderName}}, nil
	}
	res := createFormResult{Provider: spritesProviderName, Name: "x", Running: true, Public: true, API: false}
	_, err := finalizeCreateFormResult(lookup, createFlags{}, res, createFormAPIRaw{})
	if err == nil || !strings.Contains(err.Error(), "tmux terminal") {
		t.Fatalf("err = %v", err)
	}
}

func TestFinalizeCreateFormResultWithAPIParsesCommand(t *testing.T) {
	t.Parallel()
	lookup := func(string) (sandbox.Provider, error) {
		return formE2BFake{formBareProvider{name: e2bProviderName}}, nil
	}
	res := createFormResult{Provider: e2bProviderName, Name: "svc", API: true}
	raw := createFormAPIRaw{command: "python,-m,http.server", port: "9000", env: ""}
	got, err := finalizeCreateFormResult(lookup, createFlags{}, res, raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.APIPort != 9000 || len(got.APICmd) != 3 || got.APICmd[0] != "python" {
		t.Fatalf("got = %+v", got)
	}
}

func TestParsedAPIPortTmuxConflict(t *testing.T) {
	t.Parallel()
	_, err := parsedAPIPort(strconv.Itoa(webtmuxHTTPPort), true)
	if err == nil || !strings.Contains(err.Error(), "tmux") {
		t.Fatalf("err = %v", err)
	}
}

func TestFormBareProviderStubs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p := formBareProvider{name: formModalProvider}
	if _, err := p.Create(ctx, sandbox.Spec{}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.List(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Inspect(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	if err := p.Exec(ctx, "x", sandbox.ExecOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := p.Destroy(ctx, "x"); err != nil {
		t.Fatal(err)
	}
}

func TestFormRunErrMapsUserAborted(t *testing.T) {
	t.Parallel()
	err := formRunErr(huh.ErrUserAborted)
	if !errors.Is(err, errCreateCancelled) {
		t.Fatalf("err = %v", err)
	}
	if formRunErr(errors.New("other")) == nil {
		t.Fatal("expected pass-through")
	}
}

func TestCheckCreateFormFlagsEachUnsupported(t *testing.T) {
	t.Parallel()

	modalP := formModalFake{formBareProvider{name: formModalProvider}}
	e2bP := formE2BFake{formBareProvider{name: e2bProviderName}}

	if err := checkCreateFormFlags(modalP, createFlags{Running: true}); err == nil {
		t.Fatal("expected running unsupported on modal")
	}
	if err := checkCreateFormFlags(modalP, createFlags{Template: "x"}); err == nil {
		t.Fatal("expected template unsupported on modal")
	}
	if err := checkCreateFormFlags(modalP, createFlags{API: true}); err == nil {
		t.Fatal("expected api unsupported on modal")
	}
	if err := checkCreateFormFlags(modalP, createFlags{Public: true}); err == nil {
		t.Fatal("expected public unsupported on modal")
	}
	if err := checkCreateFormFlags(e2bP, createFlags{}); err != nil {
		t.Fatalf("e2b supports flags: %v", err)
	}
}

func TestFinalizeCreateFormResultAPIParseErrors(t *testing.T) {
	t.Parallel()

	lookup := func(string) (sandbox.Provider, error) {
		return formE2BFake{formBareProvider{name: e2bProviderName}}, nil
	}
	res := createFormResult{Provider: e2bProviderName, Name: "x", API: true, Running: true}
	_, err := finalizeCreateFormResult(lookup, createFlags{}, res, createFormAPIRaw{command: "", port: "8000"})
	if err == nil {
		t.Fatal("expected empty api command error")
	}
	_, err = finalizeCreateFormResult(lookup, createFlags{}, res, createFormAPIRaw{command: "x", port: "0"})
	if err == nil {
		t.Fatal("expected port error")
	}
}

func TestValidateCreateRunningPublicOKWhenAPI(t *testing.T) {
	t.Parallel()

	r := &createFormResult{Public: true, API: true}
	if err := validateCreateRunning(true, r); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCreatePublicOKWhenNotRunning(t *testing.T) {
	t.Parallel()

	r := &createFormResult{Running: false, API: false}
	if err := validateCreatePublic(true, r); err != nil {
		t.Fatal(err)
	}
}
