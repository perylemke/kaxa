package cli

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/perylemke/sbox/internal/sandbox"
)

func TestSandboxPickMovesAndConfirms(t *testing.T) {
	t.Parallel()

	spec := sandboxActionSpec{verb: "restart", title: "Restart"}
	m := newSandboxPickModel(spec, []sandbox.Info{
		{ID: "pery", Status: "warm"},
		{ID: "other", Status: "cold"},
	})
	m = pressSandboxPick(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = pressSandboxPick(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.cancelled || m.selected != "other" {
		t.Fatalf("cancelled=%v selected=%q", m.cancelled, m.selected)
	}
	view := m.View()
	if !strings.Contains(view, "Restart sandbox") || !strings.Contains(view, "other") || !strings.Contains(view, "enter restart") {
		t.Fatalf("view = %q", view)
	}
}

func TestSandboxPickCancel(t *testing.T) {
	t.Parallel()

	m := newSandboxPickModel(sandboxActionSpec{verb: "upgrade", title: "Upgrade"}, []sandbox.Info{
		{ID: "pery", Status: "warm"},
	})
	m = pressSandboxPick(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if !m.cancelled || m.selected != "" {
		t.Fatalf("cancelled=%v selected=%q", m.cancelled, m.selected)
	}
	if !strings.Contains(m.View(), "Upgrade sandbox") || !strings.Contains(m.View(), "enter upgrade") {
		t.Fatalf("view = %q", m.View())
	}
}

func pressSandboxPick(t *testing.T, m sandboxPickModel, msg tea.Msg) sandboxPickModel {
	t.Helper()
	next, _ := m.Update(msg)
	updated, ok := next.(sandboxPickModel)
	if !ok {
		t.Fatalf("model type %T", next)
	}
	return updated
}
