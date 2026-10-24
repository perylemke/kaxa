package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/perylemke/sbox/internal/sandbox"
)

type destroyCheckModel struct {
	infos     []sandbox.Info
	cursor    int
	checked   map[int]bool
	done      bool
	cancelled bool
}

func newDestroyCheckModel(infos []sandbox.Info) destroyCheckModel {
	return destroyCheckModel{
		infos:   infos,
		checked: map[int]bool{},
	}
}

func (m destroyCheckModel) Init() tea.Cmd { return nil }

func (m destroyCheckModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	case " ":
		m.checked[m.cursor] = !m.checked[m.cursor]
	case "enter":
		m.done = true
		if len(m.selected()) == 0 {
			m.cancelled = true
		}
		return m, tea.Quit
	case "q", "esc", keyInterrupt:
		m.cancelled = true
		return m, tea.Quit
	}
	return m, nil
}

func (m destroyCheckModel) View() string {
	var b strings.Builder
	b.WriteString("Destroy sandboxes\n\n")
	for i, info := range m.infos {
		mark := " "
		if m.checked[i] {
			mark = "x"
		}
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}
		fmt.Fprintf(&b, "%s [%s] %s  %s\n", cursor, mark, info.ID, info.Status)
	}
	b.WriteString("\nspace toggle • enter confirm • q cancel\n")
	return b.String()
}

func (m destroyCheckModel) selected() []sandbox.ID {
	ids := make([]sandbox.ID, 0, len(m.checked))
	for i, info := range m.infos {
		if m.checked[i] {
			ids = append(ids, info.ID)
		}
	}
	return ids
}

func inputIsTTY(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

func runDestroyCheckboxes(infos []sandbox.Info) ([]sandbox.ID, error) {
	program := tea.NewProgram(newDestroyCheckModel(infos), tea.WithAltScreen())
	final, err := program.Run()
	if err != nil {
		return nil, err
	}
	model, ok := final.(destroyCheckModel)
	if !ok {
		return nil, errors.New("destroy selection failed")
	}
	if model.cancelled {
		return nil, errDestroyCancelled
	}
	return model.selected(), nil
}
