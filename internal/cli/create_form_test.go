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

func (formModalFake) SupportsCPU()    {}
func (formModalFake) SupportsMemory() {}

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

const (
	formDockerProvider = "docker"
	formBlaxelProvider = "blaxel"
)

type formDockerFake struct{ formBareProvider }

func (formDockerFake) SupportsCPU()                                       {}
func (formDockerFake) SupportsMemory()                                    {}
func (formDockerFake) SupportsTemplate()                                  {}
func (formDockerFake) PrepareWorkspace(context.Context, sandbox.ID) error { return nil }
func (formDockerFake) SyncWorkspace(context.Context, sandbox.ID, io.Reader) error {
	return nil
}

func (formDockerFake) PublishAPI(context.Context, sandbox.ID, string, int, []string, []string) error {
	return nil
}

type formBlaxelFake struct{ formBareProvider }

func (formBlaxelFake) SupportsMemory()                                    {}
func (formBlaxelFake) SupportsTemplate()                                  {}
func (formBlaxelFake) PrepareWorkspace(context.Context, sandbox.ID) error { return nil }
func (formBlaxelFake) SyncWorkspace(context.Context, sandbox.ID, io.Reader) error {
	return nil
}

func (formBlaxelFake) PublishAPI(context.Context, sandbox.ID, string, int, []string, []string) error {
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
	case formDockerProvider:
		return formDockerFake{formBareProvider{name: formDockerProvider}}, nil
	case formBlaxelProvider:
		return formBlaxelFake{formBareProvider{name: formBlaxelProvider}}, nil
	default:
		return nil, errors.New("unknown provider")
	}
}

func formProviders() []string {
	return []string{formModalProvider, spritesProviderName, e2bProviderName, formDockerProvider, formBlaxelProvider}
}

func TestCreateFormModalAccessible(t *testing.T) {
	// modal=1, name, sync=n, CPU default, Memory default (each select needs one line)
	in := strings.NewReader(strings.Join([]string{"1", fixtureDemo, "n", "1", "1"}, "\n"))
	out := new(bytes.Buffer)

	got, err := collectCreateForm(context.Background(), in, out, formProviders(), spritesProviderName, formWizardLookup, createFlags{}, false)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if got.Name != fixtureDemo || got.Provider != formModalProvider || got.Running || got.Sync || got.API || got.Public {
		t.Fatalf("result = %+v", got)
	}
	view := out.String()
	for _, title := range []string{"Name", "Provider", "Sync", "CPU", "Memory"} {
		if !strings.Contains(view, title) {
			t.Fatalf("prompts missing %q: %q", title, view)
		}
	}
	for _, title := range []string{"Running", "Public", "API", "Template", "Block all network"} {
		if strings.Contains(view, title) {
			t.Fatalf("prompts should not contain %q: %q", title, view)
		}
	}
}

func TestCreateFormSpritesAccessibleDefaults(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{"", fixturePery, "", "", "", "", "", ""}, "\n"))
	out := new(bytes.Buffer)

	got, err := collectCreateForm(context.Background(), in, out, formProviders(), spritesProviderName, formWizardLookup, createFlags{Running: true, Sync: true}, false)
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
	if strings.Contains(view, "Block all network") {
		t.Fatalf("prompts should not contain network block: %q", view)
	}
}

func TestCreateFormE2BAccessible(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	in := strings.NewReader(strings.Join([]string{
		"3", fixturePery, "y", "", "my-template", "y", "n",
		".venv/bin/uvicorn,app.main:app", "9000", "DATABASE_URL",
	}, "\n"))
	out := new(bytes.Buffer)

	got, err := collectCreateForm(context.Background(), in, out, formProviders(), spritesProviderName, formWizardLookup, createFlags{}, false)
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
		[]string{"Template", "Sync", "API", "Directories", "Skip install", "API command", "API port", "API env"},
		[]string{"Running", "Public", "Size", "Block all network"},
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
		"2", fixturePery, "n", "y", "", "n", "n", "y",
	}, "\n"))
	out := new(bytes.Buffer)

	got, err := collectCreateForm(context.Background(), in, out, formProviders(), spritesProviderName, formWizardLookup, createFlags{}, false)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if !got.Sync || !got.NoInstall || got.DirSet {
		t.Fatalf("sync = %+v", got)
	}
	if !strings.Contains(out.String(), "Directories") || !strings.Contains(out.String(), "Skip install") {
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

	_, err := collectCreateForm(context.Background(), in, out, formProviders(), spritesProviderName, formWizardLookup, createFlags{}, false)
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

	_, err := collectCreateForm(context.Background(), strings.NewReader(""), new(bytes.Buffer), nil, "", formWizardLookup, createFlags{}, false)
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
		context.Background(),
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
	if err := checkCreateFormFlags(e2bP, createFlags{CPU: 1}); err == nil {
		t.Fatal("expected cpu unsupported on e2b")
	}
	if err := checkCreateFormFlags(modalP, createFlags{MemoryMiB: 1024}); err != nil {
		t.Fatalf("modal supports memory: %v", err)
	}
	if err := checkCreateFormFlags(modalP, createFlags{NetworkBlockAll: true}); err == nil {
		t.Fatal("expected network block unsupported on modal")
	}
}

func TestApplyCreateFormCapabilitiesClearsCPUOnUnsupportedProvider(t *testing.T) {
	t.Parallel()
	var result createFormResult
	result.Provider = formModalProvider
	result.cpuInput = "2"
	result.CPU = 2
	result.memoryInput = "4096"
	result.MemoryMiB = 4096
	result.Provider = e2bProviderName
	applyCreateFormCapabilities(&result, sandbox.CapabilitiesOf(formE2BFake{formBareProvider{name: e2bProviderName}}))
	if result.cpuInput != "" || result.CPU != 0 || result.memoryInput != "" || result.MemoryMiB != 0 {
		t.Fatalf("cleared compute: %+v", result)
	}
	if result.cpuPreset != "" || result.memoryPreset != "" || result.dockerSizePreset != "" {
		t.Fatalf("presets = %+v %+v %+v", result.cpuPreset, result.memoryPreset, result.dockerSizePreset)
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

func TestValidateCreateCPUInput(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		raw string
		ok  bool
	}{
		{"", true},
		{"1.5", true},
		{"0", false},
		{"-1", false},
		{"x", false},
	} {
		err := validateCreateCPUInput(tt.raw)
		if tt.ok && err != nil {
			t.Fatalf("%q: %v", tt.raw, err)
		}
		if !tt.ok && err == nil {
			t.Fatalf("%q: want error", tt.raw)
		}
	}
}

func TestValidateCreateMemoryInput(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		raw string
		ok  bool
	}{
		{"", true},
		{"1024", true},
		{"0", false},
		{"-1", false},
		{"1.5", false},
	} {
		err := validateCreateMemoryInput(tt.raw)
		if tt.ok && err != nil {
			t.Fatalf("%q: %v", tt.raw, err)
		}
		if !tt.ok && err == nil {
			t.Fatalf("%q: want error", tt.raw)
		}
	}
}

func TestParsedCreateCompute(t *testing.T) {
	t.Parallel()
	cpu, mem, err := parsedCreateCompute("2", "4096")
	if err != nil || cpu != 2 || mem != 4096 {
		t.Fatalf("got %v, %d, %v", cpu, mem, err)
	}
	if _, _, err := parsedCreateCompute("0", ""); err == nil {
		t.Fatal("zero cpu should fail")
	}
	if _, _, err := parsedCreateCompute("", "0"); err == nil {
		t.Fatal("zero memory should fail")
	}
}

func TestValidateCreateComputeFlags(t *testing.T) {
	t.Parallel()
	if err := validateCreateComputeFlags(0, 0); err != nil {
		t.Fatal(err)
	}
	if validateCreateComputeFlags(-0.5, 0) == nil {
		t.Fatal("negative cpu")
	}
	if validateCreateComputeFlags(0, -1) == nil {
		t.Fatal("negative memory")
	}
}

func TestCreateFormDockerAccessibleSizeMedium(t *testing.T) {
	// docker=4, name, sync=n, template="", api=n, size medium (option 4: default=1 … medium=4)
	in := strings.NewReader(strings.Join([]string{
		"4", fixtureDemo, "n", "", "n", "4",
	}, "\n"))
	out := new(bytes.Buffer)
	got, err := collectCreateForm(context.Background(), in, out, formProviders(), spritesProviderName, formWizardLookup, createFlags{}, false)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if got.Provider != formDockerProvider || got.CPU != 4 || got.MemoryMiB != 8192 {
		t.Fatalf("result = %+v", got)
	}
}

func TestCreateFormBlaxelAccessibleMemoryPreset(t *testing.T) {
	// blaxel=5, name, sync=n, template="", api=n, memory 4096 MiB (option 4)
	in := strings.NewReader(strings.Join([]string{
		"5", fixtureDemo, "n", "", "n", "4",
	}, "\n"))
	out := new(bytes.Buffer)
	got, err := collectCreateForm(context.Background(), in, out, formProviders(), spritesProviderName, formWizardLookup, createFlags{}, false)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if got.Provider != formBlaxelProvider || got.CPU != 0 || got.MemoryMiB != 4096 {
		t.Fatalf("result = %+v", got)
	}
}

func TestCheckCreateFormFlagsDockerRejectsBadPair(t *testing.T) {
	p := formDockerFake{formBareProvider{name: formDockerProvider}}
	if err := checkCreateFormFlags(p, createFlags{CPU: 3, MemoryMiB: 4096}); err == nil {
		t.Fatal("expected error for invalid docker pair")
	}
}

func TestResolveWizardComputeDockerCustomPair(t *testing.T) {
	r := createFormResult{
		Provider:         formDockerProvider,
		dockerSizePreset: formPresetDockerCustom,
		cpuInput:         "1",
		memoryInput:      "2048",
	}
	if err := resolveWizardCompute(formDockerProvider, &r); err != nil {
		t.Fatal(err)
	}
	if r.CPU != 1 || r.MemoryMiB != 2048 {
		t.Fatalf("got %+v", r)
	}
}

func TestResolveWizardComputeModalOther(t *testing.T) {
	r := createFormResult{
		Provider:     formModalProvider,
		cpuPreset:    formPresetCPUOther,
		cpuInput:     "1.25",
		memoryPreset: formPresetMemOther,
		memoryInput:  "3072",
	}
	if err := resolveWizardCompute(formModalProvider, &r); err != nil {
		t.Fatal(err)
	}
	if r.CPU != 1.25 || r.MemoryMiB != 3072 {
		t.Fatalf("got %+v", r)
	}
}

func TestResolveWizardComputeBlaxelFlagValueOutsideList(t *testing.T) {
	r := createFormResult{
		Provider:     formBlaxelProvider,
		memoryPreset: "5120",
	}
	if err := resolveWizardCompute(formBlaxelProvider, &r); err != nil {
		t.Fatal(err)
	}
	if r.MemoryMiB != 5120 || r.CPU != 0 {
		t.Fatalf("got %+v", r)
	}
}

func TestBlaxelMemorySelectOptionsAddsExtraForNonListFlagValue(t *testing.T) {
	base := len(blaxelMemorySelectOptions(nil))
	withExtra := len(blaxelMemorySelectOptions(&createFormResult{memoryPreset: "5120"}))
	if withExtra != base+1 {
		t.Fatalf("base=%d withExtra=%d", base, withExtra)
	}
	if len(blaxelMemorySelectOptions(&createFormResult{memoryPreset: "4096"})) != base {
		t.Fatal("preset value in list should not add duplicate option")
	}
}

type formDaytonaFake struct{ formBareProvider }

func (formDaytonaFake) SupportsTemplate()     {}
func (formDaytonaFake) SupportsNetworkBlock() {}

func (formDaytonaFake) ListTemplates(context.Context) ([]sandbox.TemplateOption, error) {
	return []sandbox.TemplateOption{
		{Name: "daytona-medium", Label: "daytona-medium (2 CPUs, 4 GB memory, 8 GB disk)"},
		{Name: "daytona-small", Label: "daytona-small (1 CPU, 1 GB memory, 3 GB disk)"},
	}, nil
}

type formDaytonaCatalogError struct{ formBareProvider }

func (formDaytonaCatalogError) SupportsTemplate() {}

func (formDaytonaCatalogError) ListTemplates(context.Context) ([]sandbox.TemplateOption, error) {
	return nil, errors.New("offline")
}

func daytonaFormLookup(p sandbox.Provider) func(string) (sandbox.Provider, error) {
	return func(name string) (sandbox.Provider, error) {
		if name != formProviderDaytona {
			return nil, errors.New("unknown provider")
		}
		return p, nil
	}
}

func TestCreateFormDaytonaTemplateSelect(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{"1", fixturePery, "3", "n"}, "\n"))
	out := new(bytes.Buffer)
	p := formDaytonaFake{formBareProvider{name: formProviderDaytona}}

	got, err := collectCreateForm(context.Background(), in, out, []string{formProviderDaytona}, formProviderDaytona, daytonaFormLookup(p), createFlags{}, false)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if got.Provider != formProviderDaytona || got.Name != fixturePery || got.Template != "daytona-small" || got.NetworkBlockAll {
		t.Fatalf("result = %+v", got)
	}
	view := out.String()
	if !strings.Contains(view, "Provider default") || !strings.Contains(view, "daytona-small (1 CPU, 1 GB memory, 3 GB disk)") {
		t.Fatalf("prompts = %q", view)
	}
	if strings.Contains(view, "Empty means the provider default.") {
		t.Fatalf("text template field should stay hidden: %q", view)
	}
}

func TestCreateFormDaytonaTemplateDefaultIsEmpty(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{"1", fixturePery, "1", "n"}, "\n"))
	out := new(bytes.Buffer)
	p := formDaytonaFake{formBareProvider{name: formProviderDaytona}}

	got, err := collectCreateForm(context.Background(), in, out, []string{formProviderDaytona}, formProviderDaytona, daytonaFormLookup(p), createFlags{}, false)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if got.Template != "" {
		t.Fatalf("template = %q, want empty", got.Template)
	}
}

func TestCreateFormDaytonaTemplateFlagOutsideCatalog(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{"1", fixturePery, "4", "n"}, "\n"))
	out := new(bytes.Buffer)
	p := formDaytonaFake{formBareProvider{name: formProviderDaytona}}

	got, err := collectCreateForm(context.Background(), in, out, []string{formProviderDaytona}, formProviderDaytona, daytonaFormLookup(p), createFlags{Template: "custom-snap"}, false)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if got.Template != "custom-snap" {
		t.Fatalf("template = %q", got.Template)
	}
	if !strings.Contains(out.String(), "custom-snap") {
		t.Fatalf("prompts = %q", out.String())
	}
}

func TestCreateFormDaytonaTemplateFallsBackToText(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{"1", fixturePery, "my-snap"}, "\n"))
	out := new(bytes.Buffer)
	p := formDaytonaCatalogError{formBareProvider{name: formProviderDaytona}}

	got, err := collectCreateForm(context.Background(), in, out, []string{formProviderDaytona}, formProviderDaytona, daytonaFormLookup(p), createFlags{}, false)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if got.Template != "my-snap" {
		t.Fatalf("template = %q", got.Template)
	}
	view := out.String()
	if !strings.Contains(view, "Template") || strings.Contains(view, "daytona-small") || strings.Contains(view, "Provider default uses the Daytona image") {
		t.Fatalf("prompts = %q", view)
	}
	if strings.Contains(view, "Block all network") {
		t.Fatalf("prompts should not contain network block: %q", view)
	}
}

func TestCreateFormDaytonaNetworkBlockYes(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{"1", fixturePery, "1", "y"}, "\n"))
	out := new(bytes.Buffer)
	p := formDaytonaFake{formBareProvider{name: formProviderDaytona}}

	got, err := collectCreateForm(context.Background(), in, out, []string{formProviderDaytona}, formProviderDaytona, daytonaFormLookup(p), createFlags{}, false)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if got.Template != "" || !got.NetworkBlockAll {
		t.Fatalf("result = %+v", got)
	}
	if !strings.Contains(out.String(), "Block all network") {
		t.Fatalf("prompts = %q", out.String())
	}
}

func TestCreateFormDaytonaNetworkBlockFlagSeedsYes(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{"1", fixturePery, "1", ""}, "\n"))
	out := new(bytes.Buffer)
	p := formDaytonaFake{formBareProvider{name: formProviderDaytona}}

	got, err := collectCreateForm(context.Background(), in, out, []string{formProviderDaytona}, formProviderDaytona, daytonaFormLookup(p), createFlags{NetworkBlockAll: true}, false)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if !got.NetworkBlockAll {
		t.Fatalf("result = %+v", got)
	}
}

const formBlaxelCustomImage = "custom/img:v1"

type formBlaxelCatalogFake struct{ formBareProvider }

func (formBlaxelCatalogFake) SupportsTemplate() {}

func (formBlaxelCatalogFake) ListTemplates(context.Context) ([]sandbox.TemplateOption, error) {
	return []sandbox.TemplateOption{
		{Name: "blaxel/base-image:latest", Label: "Base (blaxel/base-image:latest, 4096 MB)"},
		{Name: "blaxel/py-app:latest", Label: "Python (blaxel/py-app:latest, 4096 MB)"},
	}, nil
}

type formBlaxelEmptyCatalog struct{ formBareProvider }

func (formBlaxelEmptyCatalog) SupportsTemplate() {}

func (formBlaxelEmptyCatalog) ListTemplates(context.Context) ([]sandbox.TemplateOption, error) {
	return nil, nil
}

type formBlaxelCatalogError struct{ formBareProvider }

func (formBlaxelCatalogError) SupportsTemplate() {}

func (formBlaxelCatalogError) ListTemplates(context.Context) ([]sandbox.TemplateOption, error) {
	return nil, errors.New("offline")
}

func blaxelFormLookup(p sandbox.Provider) func(string) (sandbox.Provider, error) {
	return func(name string) (sandbox.Provider, error) {
		if name != formProviderBlaxel {
			return nil, errors.New("unknown provider")
		}
		return p, nil
	}
}

func TestCreateFormBlaxelTemplateSelect(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{"1", fixturePery, "3"}, "\n"))
	out := new(bytes.Buffer)
	p := formBlaxelCatalogFake{formBareProvider{name: formProviderBlaxel}}

	got, err := collectCreateForm(context.Background(), in, out, []string{formProviderBlaxel}, formProviderBlaxel, blaxelFormLookup(p), createFlags{}, false)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if got.Provider != formProviderBlaxel || got.Name != fixturePery || got.Template != "blaxel/py-app:latest" {
		t.Fatalf("result = %+v", got)
	}
	view := out.String()
	if !strings.Contains(view, "Provider default") || !strings.Contains(view, "Python (blaxel/py-app:latest, 4096 MB)") {
		t.Fatalf("prompts = %q", view)
	}
	if strings.Contains(view, "Empty means the provider default.") {
		t.Fatalf("text template field should stay hidden: %q", view)
	}
}

func TestCreateFormBlaxelTemplateDefaultIsEmpty(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{"1", fixturePery, "1"}, "\n"))
	out := new(bytes.Buffer)
	p := formBlaxelCatalogFake{formBareProvider{name: formProviderBlaxel}}

	got, err := collectCreateForm(context.Background(), in, out, []string{formProviderBlaxel}, formProviderBlaxel, blaxelFormLookup(p), createFlags{}, false)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if got.Template != "" {
		t.Fatalf("template = %q, want empty", got.Template)
	}
}

func TestCreateFormBlaxelTemplateEmptyCatalogStillSelects(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{"1", fixturePery, "1"}, "\n"))
	out := new(bytes.Buffer)
	p := formBlaxelEmptyCatalog{formBareProvider{name: formProviderBlaxel}}

	got, err := collectCreateForm(context.Background(), in, out, []string{formProviderBlaxel}, formProviderBlaxel, blaxelFormLookup(p), createFlags{}, false)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if got.Template != "" {
		t.Fatalf("template = %q, want empty", got.Template)
	}
	view := out.String()
	if !strings.Contains(view, "Provider default") || strings.Contains(view, "Empty means the provider default.") {
		t.Fatalf("prompts = %q", view)
	}
}

func TestCreateFormBlaxelTemplateFlagOutsideCatalog(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{"1", fixturePery, "4"}, "\n"))
	out := new(bytes.Buffer)
	p := formBlaxelCatalogFake{formBareProvider{name: formProviderBlaxel}}

	got, err := collectCreateForm(context.Background(), in, out, []string{formProviderBlaxel}, formProviderBlaxel, blaxelFormLookup(p), createFlags{Template: formBlaxelCustomImage}, false)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if got.Template != formBlaxelCustomImage {
		t.Fatalf("template = %q", got.Template)
	}
	if !strings.Contains(out.String(), formBlaxelCustomImage) {
		t.Fatalf("prompts = %q", out.String())
	}
}

func TestCreateFormBlaxelTemplateFallsBackToText(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{"1", fixturePery, formBlaxelCustomImage}, "\n"))
	out := new(bytes.Buffer)
	p := formBlaxelCatalogError{formBareProvider{name: formProviderBlaxel}}

	got, err := collectCreateForm(context.Background(), in, out, []string{formProviderBlaxel}, formProviderBlaxel, blaxelFormLookup(p), createFlags{}, false)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if got.Template != formBlaxelCustomImage {
		t.Fatalf("template = %q", got.Template)
	}
	view := out.String()
	if !strings.Contains(view, "Template") || strings.Contains(view, "blaxel/py-app:latest") || strings.Contains(view, "Provider default uses KAXA_BLAXEL_TEMPLATE") {
		t.Fatalf("prompts = %q", view)
	}
}

const formDockerCustomImage = "ghcr.io/me/img:v1"

type formDockerCatalogFake struct{ formBareProvider }

func (formDockerCatalogFake) SupportsTemplate() {}

func (formDockerCatalogFake) ListTemplates(context.Context) ([]sandbox.TemplateOption, error) {
	return []sandbox.TemplateOption{
		{Name: "docker/sandbox-templates:claude-code", Label: "claude-code"},
		{Name: "docker/sandbox-templates:shell", Label: "shell"},
	}, nil
}

type formDockerEmptyCatalog struct{ formBareProvider }

func (formDockerEmptyCatalog) SupportsTemplate() {}

func (formDockerEmptyCatalog) ListTemplates(context.Context) ([]sandbox.TemplateOption, error) {
	return nil, nil
}

type formDockerCatalogError struct{ formBareProvider }

func (formDockerCatalogError) SupportsTemplate() {}

func (formDockerCatalogError) ListTemplates(context.Context) ([]sandbox.TemplateOption, error) {
	return nil, errors.New("offline")
}

func dockerFormLookup(p sandbox.Provider) func(string) (sandbox.Provider, error) {
	return func(name string) (sandbox.Provider, error) {
		if name != formProviderDocker {
			return nil, errors.New("unknown provider")
		}
		return p, nil
	}
}

func TestCreateFormDockerTemplateCatalog(t *testing.T) {
	catalog := formDockerCatalogFake{formBareProvider{name: formProviderDocker}}
	tests := []struct {
		name    string
		p       sandbox.Provider
		flags   createFlags
		answers []string
		want    string
		has     []string
		omit    []string
	}{
		{
			name: "select", p: catalog, answers: []string{"1", fixturePery, "3"},
			want: "docker/sandbox-templates:shell",
			has:  []string{"Provider default", "shell"}, omit: []string{"Empty means the provider default."},
		},
		{
			name: "default", p: catalog, answers: []string{"1", fixturePery, "1"},
		},
		{
			name: "empty catalog", p: formDockerEmptyCatalog{formBareProvider{name: formProviderDocker}},
			answers: []string{"1", fixturePery, "1"},
			has:     []string{"Provider default"}, omit: []string{"Empty means the provider default."},
		},
		{
			name: "flag outside catalog", p: catalog, flags: createFlags{Template: formDockerCustomImage},
			answers: []string{"1", fixturePery, "4"}, want: formDockerCustomImage, has: []string{formDockerCustomImage},
		},
		{
			name: "catalog error", p: formDockerCatalogError{formBareProvider{name: formProviderDocker}},
			answers: []string{"1", fixturePery, formDockerCustomImage}, want: formDockerCustomImage,
			has:  []string{"Template"},
			omit: []string{"docker/sandbox-templates:shell", "Provider default uses KAXA_DOCKER_TEMPLATE"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := strings.NewReader(strings.Join(tt.answers, "\n"))
			out := new(bytes.Buffer)
			got, err := collectCreateForm(context.Background(), in, out, []string{formProviderDocker}, formProviderDocker, dockerFormLookup(tt.p), tt.flags, false)
			if err != nil {
				t.Fatalf("form: %v", err)
			}
			if got.Template != tt.want {
				t.Fatalf("template = %q, want %q", got.Template, tt.want)
			}
			view := out.String()
			for _, text := range tt.has {
				if !strings.Contains(view, text) {
					t.Fatalf("prompts missing %q: %q", text, view)
				}
			}
			for _, text := range tt.omit {
				if strings.Contains(view, text) {
					t.Fatalf("prompts contain %q: %q", text, view)
				}
			}
		})
	}
}
