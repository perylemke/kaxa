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
			ID:      "demo",
			Status:  "warm",
			Version: "v9",
			Labels:  []string{"a", "b"},
		},
	})
	view := m.View()
	plain := stripANSI(view)

	for _, title := range []string{listColName, listColStatus, listColVersion, listColLabels} {
		if !strings.Contains(plain, title) {
			t.Fatalf("view missing column %q: %q", title, plain)
		}
	}
	if !strings.Contains(plain, "demo") || !strings.Contains(plain, "warm") || !strings.Contains(plain, "v9") {
		t.Fatalf("view missing row data: %q", plain)
	}
	if !strings.Contains(plain, "a,b") {
		t.Fatalf("view missing joined labels: %q", plain)
	}
}
