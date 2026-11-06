package cli

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

var errCreateCancelled = errors.New("create cancelled")

type createFormResult struct {
	Name        string
	Provider    string
	Running     bool
	Sync        bool
	API         bool
	Public      bool
	Template    string
	NoInstall   bool
	Dir         string
	DirSet      bool
	APIPort     int
	APICmd      []string
	APIEnv      []string
	cpuInput    string
	memoryInput string
	// Wizard-only presets (flags stay numeric). Modal: cpu_preset / memory_preset.
	// Docker: docker_size. Blaxel: memory_preset only.
	cpuPreset        string
	memoryPreset     string
	dockerSizePreset string
	CPU              float64
	MemoryMiB        int
}

type createFormAPIRaw struct {
	command string
	port    string
	env     string
}

func collectCreateForm(
	in io.Reader,
	out io.Writer,
	providers []string,
	defaultProvider string,
	lookup func(string) (sandbox.Provider, error),
	flags createFlags,
	portSet bool,
) (createFormResult, error) {
	if len(providers) == 0 {
		return createFormResult{}, usererr.New("no sandbox providers registered", "check kaxa installation")
	}
	if err := validateCreateFlagsPreForm(flags, portSet); err != nil {
		return createFormResult{}, err
	}

	result, apiRaw := seedCreateFormResult(defaultProvider, flags)
	pages := buildCreateFormPages(providers, lookup, flags, &result, &apiRaw)

	accessible := !inputIsTTY(in)
	reader := in
	if accessible {
		reader = newOneLineReader(in)
	}
	if err := runCreateWizard(out, reader, accessible, pages, &result); err != nil {
		return createFormResult{}, err
	}
	ensureWizardPresetDefaults(&result)
	return finalizeCreateFormResult(lookup, flags, result, apiRaw)
}

func seedCreateFormResult(defaultProvider string, flags createFlags) (createFormResult, createFormAPIRaw) {
	var result createFormResult
	if defaultProvider != "" {
		result.Provider = defaultProvider
	}
	result.Running = flags.Running
	result.Sync = flags.Sync
	result.API = flags.API
	result.Public = flags.Public
	result.Template = flags.Template
	result.NoInstall = flags.NoInstall
	if flags.DirSet {
		result.Dir = flags.Dir
		result.DirSet = true
	}
	portRaw := strconv.Itoa(flags.APIPort)
	if flags.APIPort == 0 {
		portRaw = strconv.Itoa(defaultAPIPort)
	}
	out := createFormAPIRaw{
		command: flags.APICmd,
		port:    portRaw,
		env:     strings.Join(flags.APIEnv, ","),
	}
	syncComputePresetsForDefault(defaultProvider, flags, &result)
	return result, out
}

type createFormPages struct {
	identity *huh.Group
	options  *huh.Group
	sync     *huh.Group
	api      *huh.Group
}

func buildCreateFormPages(
	providers []string,
	lookup func(string) (sandbox.Provider, error),
	flags createFlags,
	result *createFormResult,
	apiRaw *createFormAPIRaw,
) createFormPages {
	options := make([]huh.Option[string], 0, len(providers))
	for _, name := range providers {
		options = append(options, huh.NewOption(name, name))
	}

	capsOf := func() sandbox.Capabilities {
		p, err := lookup(result.Provider)
		if err != nil {
			return sandbox.Capabilities{}
		}
		return sandbox.CapabilitiesOf(p)
	}
	identity := huh.NewGroup(
		shownField(huh.NewSelect[string]().Title("Provider").Options(options...).Value(&result.Provider).
			Validate(func(name string) error {
				return validateCreateProvider(name, lookup, flags, result)
			})),
		shownField(huh.NewInput().Title("Name").Value(&result.Name).Validate(validateCreateName)),
	).Title("Create sandbox").Description("Step 1 · Provider and name")

	optionsPage := huh.NewGroup(
		conditionalField(capsOf, sandbox.FeatureRunning,
			huh.NewConfirm().Title("Running").Affirmative("Yes").Negative("No").Value(&result.Running).
				Validate(func(v bool) error { return validateCreateRunning(v, result) })),
		conditionalField(capsOf, sandbox.FeatureSync,
			huh.NewConfirm().Title("Sync").Affirmative("Yes").Negative("No").Value(&result.Sync).
				Validate(func(v bool) error { return validateCreateSyncAnswer(v, result) })),
		conditionalField(capsOf, sandbox.FeatureTemplate,
			huh.NewInput().Title("Template").Description("Empty means the provider default.").Value(&result.Template)),
		conditionalField(capsOf, sandbox.FeatureAPIPublish,
			huh.NewConfirm().Title("API").Affirmative("Yes").Negative("No").Value(&result.API).
				Validate(func(v bool) error { return validateCreateAPIAnswer(v, apiRaw) })),
		conditionalField(capsOf, sandbox.FeaturePublicURLs,
			huh.NewConfirm().Title("Public").Affirmative("Yes").Negative("No").Value(&result.Public).
				Validate(func(v bool) error { return validateCreatePublic(v, result) })),
		presetConditionalField(result, dockerComputeVisible(capsOf),
			huh.NewSelect[string]().Title("Size").
				Description("Docker Sandboxes use fixed CPU and memory pairs (see Docker docs).").
				Options(dockerSizeSelectOptions()...).
				Value(&result.dockerSizePreset)),
		presetConditionalField(result, dockerCustomCPUVisible(capsOf),
			huh.NewInput().Title("Docker CPU (custom)").Description("Whole cores only. Use with Memory below.").Value(&result.cpuInput).
				Validate(validateCreateCPUInput)),
		presetConditionalField(result, dockerCustomMemVisible(capsOf),
			huh.NewInput().Title("Docker memory (custom MiB)").Value(&result.memoryInput).
				Validate(validateCreateMemoryInput)),
		presetConditionalField(result, modalCPUPresetVisible(capsOf),
			huh.NewSelect[string]().Title("CPU").
				Description("Cores for this sandbox; Provider default leaves Modal’s normal size.").
				Options(modalCPUSelectOptions()...).
				Value(&result.cpuPreset)),
		presetConditionalField(result, modalCPUOtherVisible(capsOf),
			huh.NewInput().Title("CPU (other)").Description("Fractional cores allowed.").Value(&result.cpuInput).
				Validate(validateCreateCPUInput)),
		presetConditionalField(result, modalMemPresetVisible(capsOf),
			huh.NewSelect[string]().Title("Memory (MiB)").
				Description("Provider default lets Modal choose the size.").
				Options(modalMemorySelectOptions()...).
				Value(&result.memoryPreset)),
		presetConditionalField(result, modalMemOtherVisible(capsOf),
			huh.NewInput().Title("Memory / other (MiB)").Value(&result.memoryInput).
				Validate(validateCreateMemoryInput)),
		presetConditionalField(result, blaxelMemPresetVisible(capsOf),
			huh.NewSelect[string]().Title("Memory (MiB)").
				Description("Provider default uses KAXA_BLAXEL_MEMORY or 4096.").
				Options(blaxelMemorySelectOptions(result)...).
				Value(&result.memoryPreset)),
		// Providers with CPU or memory not covered above: freeform fallback.
		presetConditionalField(result, genericCPUVisible(capsOf),
			huh.NewInput().Title("CPU (cores)").Description("Empty means the provider default.").Value(&result.cpuInput).
				Validate(validateCreateCPUInput)),
		presetConditionalField(result, genericMemVisible(capsOf),
			huh.NewInput().Title("Memory (MiB)").Description("Empty means the provider default.").Value(&result.memoryInput).
				Validate(validateCreateMemoryInput)),
	).Title("Provider options").Description("Step 2 · Options this provider supports")

	syncPage := huh.NewGroup(
		shownField(huh.NewInput().Title("Directory").Description("Empty means the current directory.").Value(&result.Dir).
			Validate(validateCreateSyncDir)),
		shownField(huh.NewConfirm().Title("Skip install").Affirmative("Yes").Negative("No").Value(&result.NoInstall)),
	).Title("Sync").Description("Step 3 · Repository")

	apiPage := huh.NewGroup(
		shownField(huh.NewInput().Title("API command").Description("Comma-separated argv. The first item is the binary.").Value(&apiRaw.command).
			Validate(func(s string) error {
				_, err := parseAPICommand(s)
				if err != nil {
					return errors.New(err.Error())
				}
				return nil
			})),
		shownField(huh.NewInput().Title("API port").Value(&apiRaw.port).Validate(func(s string) error {
			return validateAPIPortInput(s, result.Running)
		})),
		shownField(huh.NewInput().Title("API env").Description("Optional. Comma-separated KEY or KEY=VALUE entries.").Value(&apiRaw.env).
			Validate(validateCreateAPIEnvInput)),
	).Title("API").Description("Step 4 · Service")

	return createFormPages{identity: identity, options: optionsPage, sync: syncPage, api: apiPage}
}

func validateCreateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("name is required")
	}
	return nil
}

func validateCreateProvider(
	name string,
	lookup func(string) (sandbox.Provider, error),
	flags createFlags,
	result *createFormResult,
) error {
	p, err := lookup(name)
	if err != nil {
		return err
	}
	if err := checkCreateFormFlags(p, flags); err != nil {
		return err
	}
	applyCreateFormCapabilities(result, sandbox.CapabilitiesOf(p))
	syncComputePresetsAfterProviderChange(p, flags, result)
	return nil
}

func validateCreateRunning(v bool, result *createFormResult) error {
	if v && result.Public && !result.API {
		return errors.New(sandbox.PublicTerminalError().Error())
	}
	return nil
}

func validateCreateSyncAnswer(v bool, result *createFormResult) error {
	if !v {
		result.Dir = ""
		result.DirSet = false
		result.NoInstall = false
	}
	return nil
}

func validateCreateAPIAnswer(v bool, apiRaw *createFormAPIRaw) error {
	if !v {
		apiRaw.command = ""
		apiRaw.port = strconv.Itoa(defaultAPIPort)
		apiRaw.env = ""
	}
	return nil
}

func validateCreatePublic(v bool, result *createFormResult) error {
	if v && result.Running && !result.API {
		return errors.New(sandbox.PublicTerminalError().Error())
	}
	return nil
}

func validateCreateCPUInput(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	x, err := strconv.ParseFloat(s, 64)
	if err != nil || x <= 0 {
		return errors.New("use a number greater than zero, or leave empty for the provider default")
	}
	return nil
}

func validateCreateMemoryInput(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return errors.New("use a whole number of MiB greater than zero, or leave empty for the provider default")
	}
	return nil
}

func parsedCreateCompute(cpuRaw, memRaw string) (cpu float64, memMiB int, err error) {
	cpuRaw, memRaw = strings.TrimSpace(cpuRaw), strings.TrimSpace(memRaw)
	if cpuRaw != "" {
		x, err := strconv.ParseFloat(cpuRaw, 64)
		if err != nil || x <= 0 {
			return 0, 0, usererr.New(
				"cpu must be a number greater than zero",
				"omit the flag or use a positive fractional or whole number of cores",
			)
		}
		cpu = x
	}
	if memRaw != "" {
		n, err := strconv.Atoi(memRaw)
		if err != nil || n < 1 {
			return 0, 0, usererr.New(
				"memory must be a whole number of MiB greater than zero",
				"omit the flag or pass a positive size in MiB",
			)
		}
		memMiB = n
	}
	return cpu, memMiB, nil
}

func validateCreateSyncDir(dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	_, _, err := lifecycle.ResolveRepo(dir, true)
	return err
}

func validateCreateAPIEnvInput(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	_, err := lifecycle.ResolveAPIEnv(splitAPIEnvField(raw), os.LookupEnv)
	return err
}

func finalizeCreateFormResult(
	lookup func(string) (sandbox.Provider, error),
	flags createFlags,
	result createFormResult,
	apiRaw createFormAPIRaw,
) (createFormResult, error) {
	p, err := lookup(result.Provider)
	if err != nil {
		return createFormResult{}, err
	}
	applyCreateFormCapabilities(&result, sandbox.CapabilitiesOf(p))
	if err := checkCreateFormFlags(p, flags); err != nil {
		return createFormResult{}, err
	}
	if result.Public && result.Running && !result.API {
		return createFormResult{}, sandbox.PublicTerminalError()
	}

	if err := resolveWizardCompute(result.Provider, &result); err != nil {
		return createFormResult{}, err
	}

	result.Name = strings.TrimSpace(result.Name)
	result.Template = strings.TrimSpace(result.Template)
	if trimmed := strings.TrimSpace(result.Dir); trimmed != "" {
		result.Dir = trimmed
		result.DirSet = true
	} else {
		result.Dir = ""
		result.DirSet = false
	}

	if !result.API {
		return result, nil
	}
	argv, err := parseAPICommand(apiRaw.command)
	if err != nil {
		return createFormResult{}, err
	}
	port, err := parsedAPIPort(apiRaw.port, result.Running)
	if err != nil {
		return createFormResult{}, err
	}
	env, err := lifecycle.ResolveAPIEnv(splitAPIEnvField(apiRaw.env), os.LookupEnv)
	if err != nil {
		return createFormResult{}, err
	}
	result.APICmd = argv
	result.APIPort = port
	result.APIEnv = env
	return result, nil
}

// createWizardExtraTeaOpts is appended by tests (e.g. WithoutRenderer) so non-accessible wizard runs without a PTY.
var createWizardExtraTeaOpts []tea.ProgramOption

func runCreateWizard(out io.Writer, in io.Reader, accessible bool, pages createFormPages, result *createFormResult) error {
	if accessible {
		return runAccessibleCreateWizard(out, in, pages, result)
	}
	pages.sync = pages.sync.WithHideFunc(func() bool { return !result.Sync })
	pages.api = pages.api.WithHideFunc(func() bool { return !result.API })
	form := newCreateWizardForm(pages.identity, pages.options, pages.sync, pages.api).WithProgramOptions(createWizardExtraTeaOpts...)
	if err := form.Run(); err != nil {
		return formRunErr(err)
	}
	return nil
}

func runAccessibleCreateWizard(out io.Writer, in io.Reader, pages createFormPages, result *createFormResult) error {
	for _, g := range []*huh.Group{pages.identity, pages.options} {
		if err := runAccessibleCreateForm(out, in, g); err != nil {
			return err
		}
	}
	if result.Sync {
		if err := runAccessibleCreateForm(out, in, pages.sync); err != nil {
			return err
		}
	}
	if result.API {
		if err := runAccessibleCreateForm(out, in, pages.api); err != nil {
			return err
		}
	}
	return nil
}

func runAccessibleCreateForm(out io.Writer, in io.Reader, group *huh.Group) error {
	form := newThemedForm(group).WithAccessible(true).WithInput(in).WithOutput(out)
	if err := form.Run(); err != nil {
		return formRunErr(err)
	}
	return nil
}

func validateCreateComputeFlags(cpu float64, memoryMiB int) error {
	if cpu != 0 && cpu <= 0 {
		return usererr.New("--cpu must be greater than zero", "omit --cpu or pass a positive number of cores")
	}
	if memoryMiB != 0 && memoryMiB < 1 {
		return usererr.New("--memory must be a positive whole number of MiB", "omit --memory or pass MiB greater than zero")
	}
	return nil
}

func validateCreateFlagsPreForm(flags createFlags, portSet bool) error {
	if err := validateCreateComputeFlags(flags.CPU, flags.MemoryMiB); err != nil {
		return err
	}
	if !flags.API {
		if flags.APICmd != "" {
			return usererr.New("--api-cmd requires --api", "pass --api to publish the service")
		}
		if portSet {
			return usererr.New("--api-port requires --api", "pass --api to publish the service")
		}
		if len(flags.APIEnv) > 0 {
			return usererr.New("--api-env requires --api", "pass --api to publish the service")
		}
	}
	if !flags.Sync {
		if flags.DirSet {
			return usererr.New("--dir requires --sync", "pass --sync to copy that repository")
		}
		if flags.NoInstall {
			return usererr.New(
				"--no-install requires --sync",
				"pass --sync to copy the repository without installing its dependencies",
			)
		}
	}
	return nil
}

func checkCreateFormFeatureFlags(p sandbox.Provider, caps sandbox.Capabilities, flags createFlags) error {
	if flags.Running && !caps.Has(sandbox.FeatureRunning) {
		return lifecycle.Unsupported(p, sandbox.FeatureRunning)
	}
	if flags.Template != "" && !caps.Has(sandbox.FeatureTemplate) {
		return lifecycle.Unsupported(p, sandbox.FeatureTemplate)
	}
	if flags.API && !caps.Has(sandbox.FeatureAPIPublish) {
		return lifecycle.Unsupported(p, sandbox.FeatureAPIPublish)
	}
	if flags.Public && !caps.Has(sandbox.FeaturePublicURLs) {
		return lifecycle.Unsupported(p, sandbox.FeaturePublicURLs)
	}
	if flags.CPU != 0 && !caps.Has(sandbox.FeatureCPU) {
		return lifecycle.Unsupported(p, sandbox.FeatureCPU)
	}
	if flags.MemoryMiB != 0 && !caps.Has(sandbox.FeatureMemory) {
		return lifecycle.Unsupported(p, sandbox.FeatureMemory)
	}
	return nil
}

func checkDockerFormComputeFlagPairs(flags createFlags) error {
	if flags.CPU == 0 && flags.MemoryMiB == 0 {
		return nil
	}
	cpu, mem := flags.CPU, flags.MemoryMiB
	if mem == 0 {
		mem = 4096
	}
	if cpu == 0 {
		cpu = 2
	}
	if dockerSizeKeyFor(float64(cpu), mem) == "" {
		return usererr.New(
			"Docker accepts only fixed CPU and memory pairs",
			"omit --cpu and --memory, or use a pair such as 2 CPUs with 4096 MiB (see Docker Sandboxes docs)",
		)
	}
	return nil
}

func checkCreateFormFlags(p sandbox.Provider, flags createFlags) error {
	caps := sandbox.CapabilitiesOf(p)
	if err := checkCreateFormFeatureFlags(p, caps, flags); err != nil {
		return err
	}
	if p.Name() != formProviderDocker {
		return nil
	}
	return checkDockerFormComputeFlagPairs(flags)
}

func applyCreateFormBooleanCaps(result *createFormResult, caps sandbox.Capabilities) {
	if !caps.Has(sandbox.FeatureRunning) {
		result.Running = false
	}
	if !caps.Has(sandbox.FeatureSync) {
		result.Sync = false
	}
	if !caps.Has(sandbox.FeatureTemplate) {
		result.Template = ""
	}
	if !caps.Has(sandbox.FeatureAPIPublish) {
		result.API = false
	}
	if !caps.Has(sandbox.FeaturePublicURLs) {
		result.Public = false
	}
}

func clearCreateFormComputeForCaps(result *createFormResult, hasCPU, hasMem bool) {
	if !hasCPU {
		result.cpuInput = ""
		result.cpuPreset = ""
		result.CPU = 0
	}
	if !hasMem {
		result.memoryInput = ""
		result.memoryPreset = ""
		result.MemoryMiB = 0
	}
	if !hasCPU || !hasMem {
		result.dockerSizePreset = ""
	}
}

func clearWizardPresetsForProviderForm(result *createFormResult) {
	switch result.Provider {
	case formProviderDocker:
		result.cpuPreset = ""
		result.memoryPreset = ""
	case formProviderModal:
		result.dockerSizePreset = ""
	case formProviderBlaxel:
		result.dockerSizePreset = ""
		result.cpuPreset = ""
		result.cpuInput = ""
		result.CPU = 0
	default:
		result.cpuPreset = ""
		result.memoryPreset = ""
		result.dockerSizePreset = ""
	}
}

func applyCreateFormCapabilities(result *createFormResult, caps sandbox.Capabilities) {
	applyCreateFormBooleanCaps(result, caps)
	hasCPU := caps.Has(sandbox.FeatureCPU)
	hasMem := caps.Has(sandbox.FeatureMemory)
	clearCreateFormComputeForCaps(result, hasCPU, hasMem)
	clearWizardPresetsForProviderForm(result)
	if !result.Sync {
		result.Dir = ""
		result.DirSet = false
		result.NoInstall = false
	}
}

func splitAPIEnvField(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func shownField(field huh.Field) huh.Field {
	return &capabilityField{inner: field, show: func() bool { return true }}
}

func conditionalField(capsOf func() sandbox.Capabilities, feature sandbox.Feature, field huh.Field) huh.Field {
	return &capabilityField{
		inner: field,
		show: func() bool {
			return capsOf().Has(feature)
		},
	}
}

type capabilityField struct {
	inner    huh.Field
	show     func() bool
	fieldPos huh.FieldPosition
}

func (c *capabilityField) visible() bool { return c.show() }

func (c *capabilityField) viewSeparatorPrefix() string {
	if !c.visible() || c.fieldPos.Field == c.fieldPos.FirstField {
		return ""
	}
	return "\n\n"
}

func (c *capabilityField) Init() tea.Cmd { return c.inner.Init() }

func (c *capabilityField) Update(msg tea.Msg) (huh.Model, tea.Cmd) {
	m, cmd := c.inner.Update(msg)
	field, ok := m.(huh.Field)
	if ok {
		c.inner = field
	}
	return c, cmd
}

func (c *capabilityField) View() string {
	if !c.visible() {
		return ""
	}
	return c.viewSeparatorPrefix() + c.inner.View()
}

func (c *capabilityField) Blur() tea.Cmd  { return c.inner.Blur() }
func (c *capabilityField) Focus() tea.Cmd { return c.inner.Focus() }

func (c *capabilityField) Error() error { return c.inner.Error() }

func (c *capabilityField) Run() error {
	if !c.visible() {
		return nil
	}
	return c.inner.Run()
}

func (c *capabilityField) RunAccessible(w io.Writer, r io.Reader) error {
	if !c.visible() {
		return nil
	}
	return c.inner.RunAccessible(w, r)
}

func (c *capabilityField) Skip() bool {
	if !c.visible() {
		return true
	}
	return c.inner.Skip()
}

func (c *capabilityField) Zoom() bool { return c.inner.Zoom() }

func (c *capabilityField) KeyBinds() []key.Binding {
	return append(c.inner.KeyBinds(), key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")))
}

func (c *capabilityField) WithTheme(t huh.Theme) huh.Field {
	c.inner = c.inner.WithTheme(t)
	return c
}

func (c *capabilityField) WithKeyMap(k *huh.KeyMap) huh.Field {
	c.inner = c.inner.WithKeyMap(k)
	return c
}

func (c *capabilityField) WithWidth(width int) huh.Field {
	c.inner = c.inner.WithWidth(width)
	return c
}

func (c *capabilityField) WithHeight(height int) huh.Field {
	c.inner = c.inner.WithHeight(height)
	return c
}

func (c *capabilityField) WithPosition(p huh.FieldPosition) huh.Field {
	c.fieldPos = p
	c.inner = c.inner.WithPosition(p)
	return c
}

func (c *capabilityField) GetKey() string { return c.inner.GetKey() }

func (c *capabilityField) GetValue() any { return c.inner.GetValue() }

// presetField is like capabilityField but reads visibility from the form result (provider name and presets).
type presetField struct {
	result   *createFormResult
	inner    huh.Field
	show     func(*createFormResult) bool
	fieldPos huh.FieldPosition
}

func presetConditionalField(result *createFormResult, show func(*createFormResult) bool, field huh.Field) huh.Field {
	return &presetField{result: result, inner: field, show: show}
}

func (c *presetField) visible() bool {
	if c.show == nil || c.result == nil {
		return true
	}
	return c.show(c.result)
}

func (c *presetField) viewSeparatorPrefix() string {
	if !c.visible() || c.fieldPos.Field == c.fieldPos.FirstField {
		return ""
	}
	return "\n\n"
}

func (c *presetField) Init() tea.Cmd { return c.inner.Init() }

func (c *presetField) Update(msg tea.Msg) (huh.Model, tea.Cmd) {
	m, cmd := c.inner.Update(msg)
	field, ok := m.(huh.Field)
	if ok {
		c.inner = field
	}
	return c, cmd
}

func (c *presetField) View() string {
	if !c.visible() {
		return ""
	}
	return c.viewSeparatorPrefix() + c.inner.View()
}

func (c *presetField) Blur() tea.Cmd  { return c.inner.Blur() }
func (c *presetField) Focus() tea.Cmd { return c.inner.Focus() }

func (c *presetField) Error() error { return c.inner.Error() }

func (c *presetField) Run() error {
	if !c.visible() {
		return nil
	}
	return c.inner.Run()
}

func (c *presetField) RunAccessible(w io.Writer, r io.Reader) error {
	if !c.visible() {
		return nil
	}
	return c.inner.RunAccessible(w, r)
}

func (c *presetField) Skip() bool {
	if !c.visible() {
		return true
	}
	return c.inner.Skip()
}

func (c *presetField) Zoom() bool { return c.inner.Zoom() }

func (c *presetField) KeyBinds() []key.Binding {
	return append(c.inner.KeyBinds(), key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")))
}

func (c *presetField) WithTheme(t huh.Theme) huh.Field {
	c.inner = c.inner.WithTheme(t)
	return c
}

func (c *presetField) WithKeyMap(k *huh.KeyMap) huh.Field {
	c.inner = c.inner.WithKeyMap(k)
	return c
}

func (c *presetField) WithWidth(width int) huh.Field {
	c.inner = c.inner.WithWidth(width)
	return c
}

func (c *presetField) WithHeight(height int) huh.Field {
	c.inner = c.inner.WithHeight(height)
	return c
}

func (c *presetField) WithPosition(p huh.FieldPosition) huh.Field {
	c.fieldPos = p
	c.inner = c.inner.WithPosition(p)
	return c
}

func (c *presetField) GetKey() string { return c.inner.GetKey() }

func (c *presetField) GetValue() any { return c.inner.GetValue() }

func dockerComputeVisible(capsOf func() sandbox.Capabilities) func(*createFormResult) bool {
	return func(r *createFormResult) bool {
		if r.Provider != formProviderDocker {
			return false
		}
		c := capsOf()
		if !c.Has(sandbox.FeatureCPU) || !c.Has(sandbox.FeatureMemory) {
			return false
		}
		return r.dockerSizePreset != formPresetDockerCustom
	}
}

func dockerCustomCPUVisible(capsOf func() sandbox.Capabilities) func(*createFormResult) bool {
	return func(r *createFormResult) bool {
		if r.Provider != formProviderDocker {
			return false
		}
		c := capsOf()
		return c.Has(sandbox.FeatureCPU) && c.Has(sandbox.FeatureMemory) && r.dockerSizePreset == formPresetDockerCustom
	}
}

func dockerCustomMemVisible(capsOf func() sandbox.Capabilities) func(*createFormResult) bool {
	return dockerCustomCPUVisible(capsOf)
}

func modalCPUPresetVisible(capsOf func() sandbox.Capabilities) func(*createFormResult) bool {
	return func(r *createFormResult) bool {
		if r.Provider != formProviderModal {
			return false
		}
		return capsOf().Has(sandbox.FeatureCPU)
	}
}

func modalCPUOtherVisible(capsOf func() sandbox.Capabilities) func(*createFormResult) bool {
	return func(r *createFormResult) bool {
		if r.Provider != formProviderModal {
			return false
		}
		return capsOf().Has(sandbox.FeatureCPU) && r.cpuPreset == formPresetCPUOther
	}
}

func modalMemPresetVisible(capsOf func() sandbox.Capabilities) func(*createFormResult) bool {
	return func(r *createFormResult) bool {
		if r.Provider != formProviderModal {
			return false
		}
		return capsOf().Has(sandbox.FeatureMemory)
	}
}

func modalMemOtherVisible(capsOf func() sandbox.Capabilities) func(*createFormResult) bool {
	return func(r *createFormResult) bool {
		if r.Provider != formProviderModal {
			return false
		}
		return capsOf().Has(sandbox.FeatureMemory) && r.memoryPreset == formPresetMemOther
	}
}

func blaxelMemPresetVisible(capsOf func() sandbox.Capabilities) func(*createFormResult) bool {
	return func(r *createFormResult) bool {
		if r.Provider != formProviderBlaxel {
			return false
		}
		return capsOf().Has(sandbox.FeatureMemory)
	}
}

func genericCPUVisible(capsOf func() sandbox.Capabilities) func(*createFormResult) bool {
	return func(r *createFormResult) bool {
		if r.Provider == formProviderDocker || r.Provider == formProviderModal || r.Provider == formProviderBlaxel {
			return false
		}
		return capsOf().Has(sandbox.FeatureCPU)
	}
}

func genericMemVisible(capsOf func() sandbox.Capabilities) func(*createFormResult) bool {
	return func(r *createFormResult) bool {
		if r.Provider == formProviderDocker || r.Provider == formProviderModal || r.Provider == formProviderBlaxel {
			return false
		}
		return capsOf().Has(sandbox.FeatureMemory)
	}
}

func formRunErr(err error) error {
	if errors.Is(err, huh.ErrUserAborted) {
		return errCreateCancelled
	}
	return err
}

func validateAPIPortInput(raw string, running bool) error {
	_, err := parsedAPIPort(raw, running)
	return err
}

func parsedAPIPort(raw string, running bool) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || port < 1 || port > 65535 {
		return 0, errors.New("use a TCP port from 1 to 65535")
	}
	if running && port == webtmuxHTTPPort {
		return 0, errors.New("port 8080 is used by the tmux service")
	}
	return port, nil
}

// oneLineReader returns at most one line per Read so huh's accessible prompts,
// which each create a new buffered scanner, do not consume later answers.
type oneLineReader struct {
	r    *bufio.Reader
	rest []byte
}

func newOneLineReader(r io.Reader) *oneLineReader {
	return &oneLineReader{r: bufio.NewReader(r)}
}

func (o *oneLineReader) Read(p []byte) (int, error) {
	if len(o.rest) == 0 {
		line, err := o.r.ReadSlice('\n')
		if len(line) == 0 {
			return 0, err
		}
		o.rest = append([]byte(nil), line...)
		if err != nil && !errors.Is(err, bufio.ErrBufferFull) {
			n := copy(p, o.rest)
			o.rest = o.rest[n:]
			if len(o.rest) == 0 {
				return n, err
			}
			return n, nil
		}
	}
	n := copy(p, o.rest)
	o.rest = o.rest[n:]
	return n, nil
}

// formBareProvider is a minimal sandbox backend for create-form tests.
type formBareProvider struct{ name string }

func (p formBareProvider) Name() string { return p.name }
func (formBareProvider) Create(context.Context, sandbox.Spec) (sandbox.ID, error) {
	return "", nil
}
func (formBareProvider) List(context.Context) ([]sandbox.Info, error) { return nil, nil }
func (formBareProvider) Inspect(context.Context, sandbox.ID) (sandbox.Detail, error) {
	return sandbox.Detail{}, nil
}
func (formBareProvider) Exec(context.Context, sandbox.ID, sandbox.ExecOptions) error { return nil }
func (formBareProvider) Destroy(context.Context, sandbox.ID) error                   { return nil }
