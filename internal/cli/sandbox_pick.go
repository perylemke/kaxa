package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
)

var errSandboxPickCancelled = errors.New("sandbox pick cancelled")

type sandboxActionSpec struct {
	verb   string
	past   string
	gerund string
	title  string
}

func (s sandboxActionSpec) listTitle() string { return s.title + " sandbox" }

func (s sandboxActionSpec) choosePrompt() string { return s.title + " which sandbox? " }

func (s sandboxActionSpec) help() string { return "enter " + s.verb + " • q cancel" }

func newSandboxActionCmd(
	reg *registry.Registry,
	spec sandboxActionSpec,
	action func(context.Context, sandbox.Provider, sandbox.ID) error,
) *cobra.Command {
	return &cobra.Command{
		Use:   spec.verb + " [name]",
		Short: spec.title + " a sandbox",
		Args:  atMostOneName(spec.verb),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := providerFromCmd(cmd, reg)
			if err != nil {
				return err
			}

			id, picked, err := pickSandbox(cmd.Context(), p, args, cmd.InOrStdin(), cmd.ErrOrStderr(), spec)
			if errors.Is(err, errSandboxPickCancelled) {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), spec.verb+" cancelled")
				return err
			}
			if err != nil {
				return err
			}
			if id == "" {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "no sandboxes to "+spec.verb)
				return err
			}

			name := string(id)
			run := func(ctx context.Context) error {
				return action(ctx, p, id)
			}
			if picked {
				if err := runTaskWithSpinner(cmd.Context(), cmd.ErrOrStderr(), fmt.Sprintf("%s %s…", spec.gerund, name), run); err != nil {
					return err
				}
				return WriteSuccess(cmd.OutOrStdout(), spec.past, fmt.Sprintf("sandbox %s was %s", name, spec.past))
			}
			return runConfirmedSandboxAction(
				cmd.Context(),
				cmd.InOrStdin(),
				cmd.ErrOrStderr(),
				cmd.OutOrStdout(),
				fmt.Sprintf("%s sandbox %s?", spec.title, name),
				fmt.Sprintf("%s %s…", spec.gerund, name),
				spec.past,
				fmt.Sprintf("sandbox %s was %s", name, spec.past),
				run,
			)
		},
	}
}

func pickSandbox(ctx context.Context, p sandbox.Provider, args []string, in io.Reader, errOut io.Writer, spec sandboxActionSpec) (sandbox.ID, bool, error) {
	if len(args) == 1 {
		return sandbox.ID(args[0]), false, nil
	}
	infos, err := p.List(ctx)
	if err != nil {
		return "", false, err
	}
	if len(infos) == 0 {
		return "", false, nil
	}
	if inputIsTTY(in) {
		id, err := runSandboxPick(spec, infos)
		return id, true, err
	}
	id, err := chooseSandbox(in, errOut, spec.choosePrompt(), infos)
	return id, false, err
}

type sandboxPickModel struct {
	title     string
	help      string
	infos     []sandbox.Info
	cursor    int
	selected  sandbox.ID
	cancelled bool
}

func newSandboxPickModel(spec sandboxActionSpec, infos []sandbox.Info) sandboxPickModel {
	return sandboxPickModel{
		title: spec.listTitle(),
		help:  spec.help(),
		infos: infos,
	}
}

func (m sandboxPickModel) Init() tea.Cmd { return nil }

func (m sandboxPickModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.infos)-1 {
			m.cursor++
		}
	case keyEnter:
		if len(m.infos) == 0 {
			m.cancelled = true
			return m, tea.Quit
		}
		m.selected = m.infos[m.cursor].ID
		return m, tea.Quit
	case "q", "esc", keyInterrupt:
		m.cancelled = true
		return m, tea.Quit
	}
	return m, nil
}

func (m sandboxPickModel) View() string {
	var b strings.Builder
	b.WriteString(m.title)
	b.WriteString("\n\n")
	for i, info := range m.infos {
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}
		fmt.Fprintf(&b, "%s %s  %s\n", cursor, info.ID, info.Status)
	}
	b.WriteString("\n")
	b.WriteString(m.help)
	b.WriteString("\n")
	return b.String()
}

func runSandboxPick(spec sandboxActionSpec, infos []sandbox.Info) (sandbox.ID, error) {
	program := tea.NewProgram(newSandboxPickModel(spec, infos), tea.WithAltScreen())
	final, err := program.Run()
	if err != nil {
		return "", err
	}
	model, ok := final.(sandboxPickModel)
	if !ok {
		return "", fmt.Errorf("%s selection failed", spec.verb)
	}
	if model.cancelled {
		return "", errSandboxPickCancelled
	}
	return model.selected, nil
}
