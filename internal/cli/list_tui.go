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
	"github.com/spf13/cobra"

	"github.com/perylemke/sbox/internal/lifecycle"
	"github.com/perylemke/sbox/internal/sandbox"
)

type listPostAction int

const (
	listPostNone listPostAction = iota
	listPostRestart
	listPostUpgrade
	listPostDestroy
	listPostURLAuth
)

type listTUIResult struct {
	postAction listPostAction
	id         sandbox.ID
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
	inspect    func(sandbox.ID) tea.Cmd
	infos      []sandbox.Info
	table      table.Model
	mode       int
	detail     sandbox.Detail
	detailErr  error
	postAction listPostAction
	postID     sandbox.ID
	auth       string
}

type detailLoadedMsg struct {
	detail sandbox.Detail
	err    error
}

func newListInteractiveModel(ctx context.Context, p sandbox.Provider, infos []sandbox.Info) listInteractiveModel {
	return listInteractiveModel{
		inspect: func(id sandbox.ID) tea.Cmd {
			return inspectCmd(ctx, p, id)
		},
		infos: infos,
		table: newListTable(infos, true, listTableHeight(len(infos))),
		mode:  listModeTable,
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
		return m, m.inspect(m.infos[row].ID)
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
	switch msg.String() {
	case "esc":
		m.mode = listModeTable
		m.detailErr = nil
		return m, nil
	case "q", keyInterrupt:
		return m, tea.Quit
	case "r":
		m.postAction = listPostRestart
		m.postID = id
		return m, tea.Quit
	case "u":
		m.postAction = listPostUpgrade
		m.postID = id
		return m, tea.Quit
	case "d":
		m.postAction = listPostDestroy
		m.postID = id
		return m, tea.Quit
	case keyEnter:
		if id == "" || m.detailErr != nil {
			return m, nil
		}
		m.postAction = listPostURLAuth
		m.postID = id
		m.auth = selectedURLAuth(m.detail.Auth)
		return m, tea.Quit
	default:
		return m, nil
	}
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

func inspectCmd(ctx context.Context, p sandbox.Provider, id sandbox.ID) tea.Cmd {
	return func() tea.Msg {
		d, err := p.Inspect(ctx, id)
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
		fmt.Fprintf(&b, "Could not load detail: %v\n\n", m.detailErr)
		b.WriteString("esc back • q quit\n")
		return b.String()
	}
	d := m.detail
	fmt.Fprintf(&b, "Name          %s\n", d.ID)
	fmt.Fprintf(&b, "Status        %s\n", d.Status)
	if d.SpriteUUID != "" {
		fmt.Fprintf(&b, "ID            sprite-%s\n", d.SpriteUUID)
	}
	if d.Organization != "" {
		fmt.Fprintf(&b, "Organization  %s\n", d.Organization)
	}
	if d.URL != "" {
		fmt.Fprintf(&b, "URL           %s\n", d.URL)
	}
	if d.Auth != "" {
		fmt.Fprintf(&b, "Auth          %s\n", d.Auth)
	}
	fmt.Fprintf(&b, "Created       %s\n", formatRelativeTime(d.CreatedAt))
	fmt.Fprintf(&b, "Updated       %s\n", formatRelativeTime(d.UpdatedAt))
	if len(d.Labels) > 0 {
		fmt.Fprintf(&b, "Labels        %s\n", joinListLabels(d.Labels))
	}
	b.WriteString("\nenter auth • r restart • u upgrade • d destroy • esc back • q quit\n")
	return b.String()
}

func formatRelativeTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return fmt.Sprintf("%s ago", time.Since(t).Truncate(time.Second))
}

func runListInteractive(ctx context.Context, p sandbox.Provider, infos []sandbox.Info) (listTUIResult, error) {
	program := tea.NewProgram(newListInteractiveModel(ctx, p, infos), tea.WithAltScreen())
	final, err := program.Run()
	if err != nil {
		return listTUIResult{}, err
	}
	model, ok := final.(listInteractiveModel)
	if !ok {
		return listTUIResult{}, errors.New("list interactive failed")
	}
	return listTUIResult{postAction: model.postAction, id: model.postID, auth: model.auth}, nil
}

func handleListPostAction(cmd *cobra.Command, p sandbox.Provider, res listTUIResult) error {
	if res.postAction == listPostNone || res.id == "" {
		return nil
	}
	name := string(res.id)
	ctx := cmd.Context()
	in := cmd.InOrStdin()
	errOut := cmd.ErrOrStderr()
	out := cmd.OutOrStdout()

	switch res.postAction {
	case listPostNone:
		return nil
	case listPostRestart:
		return runConfirmedSandboxAction(ctx, in, errOut, out,
			fmt.Sprintf("Restart sandbox %s?", name),
			fmt.Sprintf("restarting %s…", name),
			"restarted",
			fmt.Sprintf("sandbox %s was restarted", name),
			func(ctx context.Context) error { return p.Restart(ctx, res.id) },
		)
	case listPostUpgrade:
		return runConfirmedSandboxAction(ctx, in, errOut, out,
			fmt.Sprintf("Upgrade sandbox %s?", name),
			fmt.Sprintf("upgrading %s…", name),
			"upgraded",
			fmt.Sprintf("sandbox %s was upgraded", name),
			func(ctx context.Context) error { return p.Upgrade(ctx, res.id) },
		)
	case listPostDestroy:
		return runConfirmedSandboxAction(ctx, in, errOut, out,
			destroyPrompt(false, []sandbox.ID{res.id}),
			destroySpinnerLabel([]sandbox.ID{res.id}),
			"destroyed",
			destroySuccessMessage([]sandbox.ID{res.id}),
			func(ctx context.Context) error { return p.Destroy(ctx, res.id) },
		)
	case listPostURLAuth:
		return runListURLAuth(ctx, in, errOut, out, p, res)
	default:
		return nil
	}
}

func runListURLAuth(ctx context.Context, in io.Reader, errOut, out io.Writer, p sandbox.Provider, res listTUIResult) error {
	setter, ok := p.(sandbox.PublicURL)
	if !ok {
		return lifecycle.Unsupported(p, "url auth")
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
	confirmed, err := confirmAction(reader, errOut, fmt.Sprintf("Set sandbox %s URL auth to %s?", res.id, auth))
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
	msg := fmt.Sprintf("sandbox %s URL auth is %s\n%s", res.id, auth, detail.URL)
	return WriteSuccess(out, "url", msg)
}
