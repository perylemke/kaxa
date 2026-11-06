package cli

import (
	"io"
	"strings"
	"testing"

	"charm.land/huh/v2"
)

func TestNewCreateWizardFormAccessibleRun(t *testing.T) {
	t.Parallel()

	name := ""
	g := huh.NewGroup(huh.NewInput().Title("Name").Value(&name))
	form := newCreateWizardForm(g).
		WithAccessible(true).
		WithInput(strings.NewReader("alice\n")).
		WithOutput(io.Discard)
	if err := form.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if name != "alice" {
		t.Fatalf("name = %q", name)
	}
}
