package cli

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/perylemke/sbox/internal/sandbox"
)

func TestDestroyCheckTogglesAndConfirms(t *testing.T) {
	t.Parallel()

	m := newDestroyCheckModel([]sandbox.Info{
		{ID: "pery", Status: "warm"},
		{ID: "other", Status: "cold"},
	})
	m = press(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = press(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.cancelled {
		t.Fatal("expected confirmation")
	}
	got := m.selected()
	if len(got) != 2 || got[0] != "pery" || got[1] != "other" {
		t.Fatalf("selected = %v", got)
	}
	view := m.View()
	if !strings.Contains(view, "[x] pery") || !strings.Contains(view, "[x] other") {
		t.Fatalf("view = %q", view)
	}
}

func TestDestroyCheckCancel(t *testing.T) {
	t.Parallel()

	m := newDestroyCheckModel([]sandbox.Info{{ID: "pery", Status: "warm"}})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if !m.cancelled || len(m.selected()) != 0 {
		t.Fatalf("cancelled=%v selected=%v", m.cancelled, m.selected())
	}
}

func press(t *testing.T, m destroyCheckModel, msg tea.Msg) destroyCheckModel {
	t.Helper()
	next, _ := m.Update(msg)
	updated, ok := next.(destroyCheckModel)
	if !ok {
		t.Fatalf("model type %T", next)
	}
	return updated
}
