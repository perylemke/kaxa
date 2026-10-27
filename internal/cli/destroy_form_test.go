package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestDestroyFormViewListsSandboxes(t *testing.T) {
	t.Parallel()

	var selected []sandbox.ID
	form := newDestroyForm([]sandbox.Info{
		{ID: fixturePery, Status: fixtureCold},
		{ID: fixtureOther, Status: fixtureWarm},
	}, &selected)
	form.Init()
	updated, _ := form.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	view := updated.View()
	if !strings.Contains(view, fixturePery) || !strings.Contains(view, fixtureOther) {
		t.Fatalf("view = %q", view)
	}
}

func TestDestroyFormAccessibleSelectsOne(t *testing.T) {
	t.Parallel()

	infos := []sandbox.Info{
		{ID: "a", Status: fixtureCold},
		{ID: "b", Status: fixtureWarm},
	}
	in := strings.NewReader("2\n0\n")
	out := new(bytes.Buffer)

	got, err := collectDestroyForm(in, out, infos)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if len(got) != 1 || got[0] != "b" {
		t.Fatalf("selected = %v", got)
	}
}

func TestDestroyFormAccessibleSelectsMultiple(t *testing.T) {
	t.Parallel()

	infos := []sandbox.Info{
		{ID: fixturePery, Status: fixtureWarm},
		{ID: fixtureOther, Status: fixtureCold},
	}
	in := strings.NewReader("1\n2\n0\n")
	out := new(bytes.Buffer)

	got, err := collectDestroyForm(in, out, infos)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if len(got) != 2 || got[0] != fixturePery || got[1] != fixtureOther {
		t.Fatalf("selected = %v", got)
	}
}

func TestDestroyFormEmptySelectionCancelled(t *testing.T) {
	t.Parallel()

	infos := []sandbox.Info{{ID: fixturePery, Status: fixtureWarm}}
	in := strings.NewReader("0\n")
	out := new(bytes.Buffer)

	_, err := collectDestroyForm(in, out, infos)
	if !errors.Is(err, errDestroyCancelled) {
		t.Fatalf("err = %v", err)
	}
}
