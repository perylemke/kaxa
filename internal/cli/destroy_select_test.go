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
		{ID: fixturePery, Status: fixtureWarm},
		{ID: fixtureOther, Status: fixtureCold},
	})
	m = press(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = press(t, m, tea.KeyMsg{Type: tea.KeySpace})
	m = press(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.cancelled {
		t.Fatal("expected confirmation")
	}
	got := m.selected()
	if len(got) != 2 || got[0] != fixturePery || got[1] != fixtureOther {
		t.Fatalf("selected = %v", got)
	}
	view := m.View()
	if !strings.Contains(view, "[x] "+fixturePery) || !strings.Contains(view, "[x] "+fixtureOther) {
		t.Fatalf("view = %q", view)
	}
}

func TestDestroyCheckCancel(t *testing.T) {
	t.Parallel()

	m := newDestroyCheckModel([]sandbox.Info{{ID: fixturePery, Status: fixtureWarm}})
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
