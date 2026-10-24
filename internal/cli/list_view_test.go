package cli

import (
	"strings"
	"testing"

	"github.com/perylemke/sbox/internal/sandbox"
)

func TestListModelViewShowsColumnsAndRow(t *testing.T) {
	t.Parallel()

	m := newListModel([]sandbox.Info{
		{
			ID:       fixtureDemo,
			Status:   fixtureWarm,
			Provider: spritesProviderName,
			Labels:   []string{"a", "b"},
		},
	})
	view := m.View()
	plain := stripANSI(view)

	for _, title := range []string{listColName, listColStatus, listColProvider, listColLabels} {
		if !strings.Contains(plain, title) {
			t.Fatalf("view missing column %q: %q", title, plain)
		}
	}
	if !strings.Contains(plain, fixtureDemo) || !strings.Contains(plain, fixtureWarm) || !strings.Contains(plain, spritesProviderName) {
		t.Fatalf("view missing row data: %q", plain)
	}
	if !strings.Contains(plain, "a,b") {
		t.Fatalf("view missing joined labels: %q", plain)
	}
}
