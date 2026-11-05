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
	Name      string
	Provider  string
	Running   bool
	Sync      bool
	API       bool
	Public    bool
	Template  string
	NoInstall bool
	Dir       string
	DirSet    bool
	APIPort   int
	APICmd    []string
	APIEnv    []string
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
	return result, createFormAPIRaw{
		command: flags.APICmd,
		port:    portRaw,
		env:     strings.Join(flags.APIEnv, ","),
	}
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
		huh.NewSelect[string]().Title("Provider").Options(options...).Value(&result.Provider).
			Validate(func(name string) error {
				return validateCreateProvider(name, lookup, flags, result)
			}),
		huh.NewInput().Title("Name").Value(&result.Name).Validate(validateCreateName),
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
	).Title("Provider options").Description("Step 2 · Options this provider supports")

	syncPage := huh.NewGroup(
		huh.NewInput().Title("Directory").Description("Empty means the current directory.").Value(&result.Dir).
			Validate(validateCreateSyncDir),
		huh.NewConfirm().Title("Skip install").Affirmative("Yes").Negative("No").Value(&result.NoInstall),
	).Title("Sync").Description("Step 3 · Repository")

	apiPage := huh.NewGroup(
		huh.NewInput().Title("API command").Description("Comma-separated argv. The first item is the binary.").Value(&apiRaw.command).
			Validate(func(s string) error {
				_, err := parseAPICommand(s)
				if err != nil {
					return errors.New(err.Error())
				}
				return nil
			}),
		huh.NewInput().Title("API port").Value(&apiRaw.port).Validate(func(s string) error {
			return validateAPIPortInput(s, result.Running)
		}),
		huh.NewInput().Title("API env").Description("Optional. Comma-separated KEY or KEY=VALUE entries.").Value(&apiRaw.env).
			Validate(validateCreateAPIEnvInput),
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

func runCreateWizard(out io.Writer, in io.Reader, accessible bool, pages createFormPages, result *createFormResult) error {
	if accessible {
		return runAccessibleCreateWizard(out, in, pages, result)
	}
	pages.sync = pages.sync.WithHideFunc(func() bool { return !result.Sync })
	pages.api = pages.api.WithHideFunc(func() bool { return !result.API })
	form := newThemedForm(pages.identity, pages.options, pages.sync, pages.api).
		WithViewHook(func(v tea.View) tea.View {
			v.ReportFocus = true
			v.AltScreen = true
			return v
		})
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

func validateCreateFlagsPreForm(flags createFlags, portSet bool) error {
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

func checkCreateFormFlags(p sandbox.Provider, flags createFlags) error {
	caps := sandbox.CapabilitiesOf(p)
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
	return nil
}

func applyCreateFormCapabilities(result *createFormResult, caps sandbox.Capabilities) {
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

func conditionalField(capsOf func() sandbox.Capabilities, feature sandbox.Feature, field huh.Field) huh.Field {
	return &capabilityField{
		inner: field,
		show: func() bool {
			return capsOf().Has(feature)
		},
	}
}

type capabilityField struct {
	inner huh.Field
	show  func() bool
}

func (c *capabilityField) visible() bool { return c.show() }

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
	return c.inner.View()
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

func (c *capabilityField) KeyBinds() []key.Binding { return c.inner.KeyBinds() }

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
	c.inner = c.inner.WithPosition(p)
	return c
}

func (c *capabilityField) GetKey() string { return c.inner.GetKey() }

func (c *capabilityField) GetValue() any { return c.inner.GetValue() }

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
