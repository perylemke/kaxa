package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/sandbox"
)

type listPostAction int

const (
	listPostNone listPostAction = iota
	listPostRestart
	listPostUpgrade
	listPostDestroy
	listPostURLAuth
	listPostConsole
)

type listTUIResult struct {
	postAction listPostAction
	id         sandbox.ID
	provider   string
	auth       string
}

const (
	listModeTable int = iota
	listModeDetailLoading
	listModeDetail
)

const (
	keyInterrupt = "ctrl+c"
	keyEnter     = "enter"
)

type listInteractiveModel struct {
	inspect    func(sandbox.Info) tea.Cmd
	providers  map[string]sandbox.Provider
	infos      []sandbox.Info
	table      table.Model
	mode       int
	detail     sandbox.Detail
	detailErr  error
	detailMsg  string
	postAction listPostAction
	postID     sandbox.ID
	postProv   string
	auth       string
}

type detailLoadedMsg struct {
	detail sandbox.Detail
	err    error
}

func newListInteractiveModel(ctx context.Context, providers map[string]sandbox.Provider, infos []sandbox.Info, startCursor int) listInteractiveModel {
	tbl := newListTable(infos, true, listTableHeight(len(infos)))
	if startCursor > 0 && startCursor < len(infos) {
		tbl.SetCursor(startCursor)
	}
	return listInteractiveModel{
		inspect: func(info sandbox.Info) tea.Cmd {
			return inspectCmd(ctx, providers[info.Provider], info)
		},
		providers: providers,
		infos:     infos,
		table:     tbl,
		mode:      listModeTable,
	}
}

func listTableHeight(rowCount int) int {
	const maxRows = 20
	h := max(rowCount+2, 4)
	if h > maxRows {
		return maxRows
	}
	return h
}

func (m listInteractiveModel) Init() tea.Cmd { return nil }

func (m listInteractiveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case detailLoadedMsg:
		return m.applyDetail(msg), nil
	case tea.KeyMsg:
		return m.updateKey(msg)
	default:
		return m, nil
	}
}

func (m listInteractiveModel) applyDetail(msg detailLoadedMsg) listInteractiveModel {
	m.mode = listModeDetail
	m.detailErr = msg.err
	m.detailMsg = ""
	if msg.err == nil {
		m.detail = msg.detail
	}
	return m
}

func (m listInteractiveModel) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case listModeTable:
		return m.updateTableKey(msg)
	case listModeDetailLoading:
		if msg.String() == "q" || msg.String() == keyInterrupt {
			return m, tea.Quit
		}
		return m, nil
	case listModeDetail:
		return m.updateDetailKey(msg)
	default:
		return m, nil
	}
}

func (m listInteractiveModel) updateTableKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyEnter:
		if len(m.infos) == 0 {
			return m, nil
		}
		row := m.table.Cursor()
		if row < 0 || row >= len(m.infos) {
			return m, nil
		}
		m.mode = listModeDetailLoading
		return m, m.inspect(m.infos[row])
	case "q", keyInterrupt:
		return m, tea.Quit
	default:
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		return m, cmd
	}
}

func (m listInteractiveModel) updateDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	id := m.detailID()
	prov := m.detailProvider()
	p := m.providers[prov]
	switch msg.String() {
	case "esc":
		m.mode = listModeTable
		m.detailErr = nil
		m.detailMsg = ""
		return m, nil
	case "q", keyInterrupt:
		return m, tea.Quit
	case "r":
		return m.detailFeatureAction(p, sandbox.FeatureRestart, listPostRestart, id, prov)
	case "u":
		return m.detailFeatureAction(p, sandbox.FeatureUpgrade, listPostUpgrade, id, prov)
	case "c":
		return m.quitForAction(listPostConsole, id, prov)
	case "d":
		return m.quitForAction(listPostDestroy, id, prov)
	case keyEnter:
		if id == "" || m.detailErr != nil {
			return m, nil
		}
		if p == nil || !sandbox.CapabilitiesOf(p).Has(sandbox.FeatureURLAuth) {
			return m.showUnsupported(p, sandbox.FeatureURLAuth), nil
		}
		m.auth = selectedURLAuth(m.detail.Auth)
		return m.quitForAction(listPostURLAuth, id, prov)
	default:
		return m, nil
	}
}

func (m listInteractiveModel) detailFeatureAction(p sandbox.Provider, feature sandbox.Feature, action listPostAction, id sandbox.ID, prov string) (tea.Model, tea.Cmd) {
	if p == nil || !sandbox.CapabilitiesOf(p).Has(feature) {
		return m.showUnsupported(p, feature), nil
	}
	return m.quitForAction(action, id, prov)
}

func (m listInteractiveModel) showUnsupported(p sandbox.Provider, feature sandbox.Feature) listInteractiveModel {
	if p != nil {
		m.detailMsg = lifecycle.Unsupported(p, feature).Error()
	}
	return m
}

func (m listInteractiveModel) quitForAction(action listPostAction, id sandbox.ID, prov string) (tea.Model, tea.Cmd) {
	m.postAction = action
	m.postID = id
	m.postProv = prov
	return m, tea.Quit
}

func selectedURLAuth(displayed string) string {
	if strings.HasPrefix(displayed, sandbox.URLAuthPublic) {
		return sandbox.URLAuthPublic
	}
	return sandbox.URLAuthSprite
}

func (m listInteractiveModel) detailID() sandbox.ID {
	if m.detail.ID != "" || m.detailErr == nil || len(m.infos) == 0 {
		return m.detail.ID
	}
	row := m.table.Cursor()
	if row < 0 || row >= len(m.infos) {
		return m.detail.ID
	}
	return m.infos[row].ID
}

// detailProvider names the provider of the sandbox whose detail is open.
func (m listInteractiveModel) detailProvider() string {
	row := m.table.Cursor()
	if row >= 0 && row < len(m.infos) {
		return m.infos[row].Provider
	}
	return m.detail.Provider
}

func inspectCmd(ctx context.Context, p sandbox.Provider, info sandbox.Info) tea.Cmd {
	return func() tea.Msg {
		if p == nil {
			return detailLoadedMsg{err: fmt.Errorf("provider %q is not configured", info.Provider)}
		}
		d, err := p.Inspect(ctx, info.ID)
		return detailLoadedMsg{detail: d, err: err}
	}
}

func (m listInteractiveModel) View() string {
	switch m.mode {
	case listModeDetailLoading:
		return "Loading detail…\n"
	case listModeDetail:
		return m.detailView()
	default:
		help := "enter detail • q quit\n"
		return m.table.View() + "\n" + help
	}
}

func (m listInteractiveModel) detailView() string {
	var b strings.Builder
	b.WriteString("Sandbox detail\n\n")
	if m.detailErr != nil {
		fmt.Fprintf(&b, "Could not load detail: %s\n\n", plainText(m.detailErr.Error()))
		b.WriteString("esc back • q quit\n")
		return b.String()
	}
	d := m.detail
	fmt.Fprintf(&b, "Name          %s\n", plainText(string(d.ID)))
	if d.Provider != "" {
		fmt.Fprintf(&b, "Provider      %s\n", plainText(d.Provider))
	}
	fmt.Fprintf(&b, "Status        %s\n", plainText(d.Status))
	for _, f := range d.Fields {
		writeDetailField(&b, f)
	}
	if len(d.Labels) > 0 {
		fmt.Fprintf(&b, "Labels        %s\n", joinListLabels(d.Labels))
	}
	if m.detailMsg != "" {
		b.WriteString("\n")
		fmt.Fprintf(&b, "%s\n", plainText(m.detailMsg))
	}
	prov := m.detailProvider()
	p := m.providers[prov]
	var caps sandbox.Capabilities
	if p != nil {
		caps = sandbox.CapabilitiesOf(p)
	}
	b.WriteString("\n")
	b.WriteString(listDetailHelp(caps))
	b.WriteString("\n")
	return b.String()
}

func writeDetailField(b *strings.Builder, f sandbox.Field) {
	var value string
	switch {
	case !f.At.IsZero():
		value = formatRelativeTime(f.At)
	case f.Text != "":
		value = f.Text
	default:
		return
	}
	fmt.Fprintf(b, "%-14s%s\n", f.Label, plainText(value))
}

func listDetailHelp(caps sandbox.Capabilities) string {
	var parts []string
	if caps.Has(sandbox.FeatureURLAuth) {
		parts = append(parts, "enter auth")
	}
	if caps.Has(sandbox.FeatureRestart) {
		parts = append(parts, "r restart")
	}
	if caps.Has(sandbox.FeatureUpgrade) {
		parts = append(parts, "u upgrade")
	}
	parts = append(parts, "c console", "d destroy", "esc back", "q quit")
	return strings.Join(parts, " • ")
}

func formatRelativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return fmt.Sprintf("%s ago", time.Since(t).Truncate(time.Second))
}

func runListInteractive(ctx context.Context, providers map[string]sandbox.Provider, infos []sandbox.Info, startCursor int) (listTUIResult, error) {
	program := tea.NewProgram(newListInteractiveModel(ctx, providers, infos, startCursor), tea.WithAltScreen())
	final, err := program.Run()
	if err != nil {
		return listTUIResult{}, err
	}
	model, ok := final.(listInteractiveModel)
	if !ok {
		return listTUIResult{}, errors.New("list interactive failed")
	}
	return listTUIResult{postAction: model.postAction, id: model.postID, provider: model.postProv, auth: model.auth}, nil
}

func runListURLAuth(ctx context.Context, in io.Reader, errOut, out io.Writer, p sandbox.Provider, res listTUIResult) error {
	setter, ok := p.(sandbox.PublicURL)
	if !ok {
		return lifecycle.Unsupported(p, sandbox.FeatureURLAuth)
	}
	reader := in
	if !inputIsTTY(in) {
		reader = newOneLineReader(in)
	}
	auth, err := collectURLAuth(reader, errOut, res.auth)
	if errors.Is(err, errURLAuthCancelled) {
		_, err = fmt.Fprintln(out, "cancelled")
		return err
	}
	if err != nil {
		return err
	}
	confirmed, err := confirmAction(reader, errOut, fmt.Sprintf("Set sandbox %s URL auth to %s?", sandboxLabel(p.Name(), res.id), auth))
	if err != nil {
		return err
	}
	if !confirmed {
		_, err = fmt.Fprintln(out, "cancelled")
		return err
	}
	err = runTaskWithSpinner(ctx, errOut, "setting url auth…", func(ctx context.Context) error {
		return setter.SetURLAuth(ctx, res.id, auth)
	})
	if err != nil {
		return err
	}
	detail, err := p.Inspect(ctx, res.id)
	if err != nil {
		return err
	}
	msg := fmt.Sprintf("sandbox %s URL auth is %s\n%s", sandboxLabel(p.Name(), res.id), auth, detail.URL)
	return WriteSuccess(out, "url", msg)
}

type listInteractiveRunner func(context.Context, map[string]sandbox.Provider, []sandbox.Info, int) (listTUIResult, error)

type listConsoleOpener func(context.Context, sandbox.Provider, sandbox.ID, io.Writer, io.Writer) error

func runListWithConsoleLoop(
	ctx context.Context,
	providers map[string]sandbox.Provider,
	infos []sandbox.Info,
	in io.Reader,
	out, errOut io.Writer,
	run listInteractiveRunner,
	open listConsoleOpener,
) error {
	cursor := 0
	for {
		res, err := run(ctx, providers, infos, cursor)
		if err != nil {
			return err
		}
		if res.postAction == listPostNone || res.id == "" {
			return nil
		}
		if res.postAction != listPostConsole {
			return handleListPostActionStreams(ctx, in, out, errOut, providers, res)
		}
		p, ok := providers[res.provider]
		if !ok {
			return fmt.Errorf("provider %q is not configured", res.provider)
		}
		if err := open(ctx, p, res.id, out, errOut); err != nil {
			_, _ = fmt.Fprintln(errOut, err)
		}
		cursor = max(0, listInfoIndex(infos, res.provider, res.id))
	}
}

func handleListPostActionStreams(ctx context.Context, in io.Reader, out, errOut io.Writer, providers map[string]sandbox.Provider, res listTUIResult) error {
	if res.postAction == listPostNone || res.id == "" {
		return nil
	}
	p, ok := providers[res.provider]
	if !ok {
		return fmt.Errorf("provider %q is not configured", res.provider)
	}
	ref := sandboxLabel(p.Name(), res.id)

	switch res.postAction {
	case listPostNone:
		return nil
	case listPostRestart:
		r, ok := p.(sandbox.Restarter)
		if !ok {
			return lifecycle.Unsupported(p, sandbox.FeatureRestart)
		}
		return runConfirmedSandboxAction(ctx, in, errOut, out,
			fmt.Sprintf("Restart sandbox %s?", ref),
			fmt.Sprintf("restarting %s…", ref),
			"restarted",
			fmt.Sprintf("sandbox %s was restarted", ref),
			func(ctx context.Context) error { return r.Restart(ctx, res.id) },
		)
	case listPostUpgrade:
		u, ok := p.(sandbox.Upgrader)
		if !ok {
			return lifecycle.Unsupported(p, sandbox.FeatureUpgrade)
		}
		return runConfirmedSandboxAction(ctx, in, errOut, out,
			fmt.Sprintf("Upgrade sandbox %s?", ref),
			fmt.Sprintf("upgrading %s…", ref),
			"upgraded",
			fmt.Sprintf("sandbox %s was upgraded", ref),
			func(ctx context.Context) error { return u.Upgrade(ctx, res.id) },
		)
	case listPostDestroy:
		target := []sandbox.Info{{Provider: p.Name(), ID: res.id}}
		return runConfirmedSandboxAction(ctx, in, errOut, out,
			destroyPrompt(target, false),
			destroySpinnerLabel(target),
			"destroyed",
			destroySuccessMessage(target),
			func(ctx context.Context) error { return p.Destroy(ctx, res.id) },
		)
	case listPostURLAuth:
		return runListURLAuth(ctx, in, errOut, out, p, res)
	case listPostConsole:
		return openSandboxConsole(ctx, p, res.id, out, errOut)
	default:
		return nil
	}
}

func listInfoIndex(infos []sandbox.Info, provider string, id sandbox.ID) int {
	for i, info := range infos {
		if info.Provider == provider && info.ID == id {
			return i
		}
	}
	return -1
}
