package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/sbox/internal/sandbox"
)

func TestListInteractiveDetailViewShowsFields(t *testing.T) {
	t.Parallel()

	m := listInteractiveModel{
		mode: listModeDetail,
		detail: sandbox.Detail{
			ID:           fixtureDemo,
			Status:       fixtureWarm,
			SpriteUUID:   "uuid-1",
			Organization: "acme",
			URL:          "https://demo.sprites.dev",
			Auth:         "sprite · admins",
			Labels:       []string{"sbox", "team=a"},
			CreatedAt:    time.Now().Add(-2 * time.Hour),
			UpdatedAt:    time.Now().Add(-5 * time.Minute),
		},
	}
	view := stripANSI(m.View())
	for _, want := range []string{
		fixtureDemo,
		fixtureWarm,
		"sprite-uuid-1",
		"acme",
		"https://demo.sprites.dev",
		"sprite · admins",
		"sbox,team=a",
		"r restart",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestListInteractiveListViewShowsHelp(t *testing.T) {
	t.Parallel()

	m := newListInteractiveModel(context.TODO(), nil, []sandbox.Info{
		{ID: "a", Status: fixtureWarm, Provider: spritesProviderName},
	})
	m.mode = listModeTable
	view := stripANSI(m.View())
	if !strings.Contains(view, "enter detail") || !strings.Contains(view, "a") {
		t.Fatalf("view = %q", view)
	}
}
