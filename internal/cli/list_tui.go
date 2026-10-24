package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/perylemke/sbox/internal/sandbox"
)

type listPostAction int

const (
	listPostNone listPostAction = iota
	listPostRestart
	listPostUpgrade
	listPostDestroy
)

type listTUIResult struct {
	postAction listPostAction
	id         sandbox.ID
}

const (
	listModeTable int = iota
	listModeDetailLoading
	listModeDetail
)

type listInteractiveModel struct {
	ctx        context.Context
	provider   sandbox.Provider
	infos      []sandbox.Info
	table      table.Model
	mode       int
	detail     sandbox.Detail
	detailErr  error
	postAction listPostAction
	postID     sandbox.ID
}

type detailLoadedMsg struct {
	detail sandbox.Detail
	err    error
}

func newListInteractiveModel(ctx context.Context, p sandbox.Provider, infos []sandbox.Info) listInteractiveModel {
	return listInteractiveModel{
		ctx:      ctx,
		provider: p,
		infos:    infos,
		table:    newListTable(infos, true, listTableHeight(len(infos))),
		mode:     listModeTable,
	}
}

func listTableHeight(rowCount int) int {
	const maxRows = 20
	h := rowCount + 2
	if h < 4 {
		h = 4
	}
	if h > maxRows {
		h = maxRows
	}
	return h
}

func (m listInteractiveModel) Init() tea.Cmd { return nil }

func (m listInteractiveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case detailLoadedMsg:
		m.mode = listModeDetail
		if msg.err != nil {
			m.detailErr = msg.err
			return m, nil
		}
		m.detail = msg.detail
		m.detailErr = nil
		return m, nil
	case tea.KeyMsg:
		switch m.mode {
		case listModeTable:
			switch msg.String() {
			case "enter":
				if len(m.infos) == 0 {
					return m, nil
				}
				row := m.table.Cursor()
				if row < 0 || row >= len(m.infos) {
					return m, nil
				}
				id := m.infos[row].ID
				m.mode = listModeDetailLoading
				return m, inspectCmd(m.ctx, m.provider, id)
			case "q", "ctrl+c":
				return m, tea.Quit
			}
			var cmd tea.Cmd
			m.table, cmd = m.table.Update(msg)
			return m, cmd
		case listModeDetailLoading:
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		case listModeDetail:
			id := m.detail.ID
			if m.detailErr != nil && id == "" {
				if len(m.infos) > 0 {
					row := m.table.Cursor()
					if row >= 0 && row < len(m.infos) {
						id = m.infos[row].ID
					}
				}
			}
			switch msg.String() {
			case "esc":
				m.mode = listModeTable
				m.detailErr = nil
				return m, nil
			case "q", "ctrl+c":
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
			}
		}
	}
	return m, nil
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
	b.WriteString("\nr restart • u upgrade • d destroy • esc back • q quit\n")
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
	return listTUIResult{postAction: model.postAction, id: model.postID}, nil
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
	default:
		return nil
	}
}
