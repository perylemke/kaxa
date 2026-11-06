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

func TestCreateWizardOptionsGroupNoHugeGapsBetweenVisibleFields(t *testing.T) {
	t.Parallel()

	result := createFormResult{Provider: formModalProvider}
	apiRaw := createFormAPIRaw{}
	pages := buildCreateFormPages(formProviders(), formWizardLookup, createFlags{}, &result, &apiRaw)
	grp := pages.options.WithTheme(huh.ThemeFunc(createWizardTheme))
	_ = huh.NewForm(grp).WithTheme(huh.ThemeFunc(createWizardTheme)).UpdateFieldPositions()

	content := grp.Content()
	if !strings.Contains(content, "Sync") || !strings.Contains(content, "CPU") {
		t.Fatalf("unexpected content (want Sync and CPU):\n%s", content)
	}
	iSync := strings.Index(content, "Sync")
	iCPU := strings.Index(content[iSync:], "CPU")
	if iCPU < 0 {
		t.Fatal("CPU not found after Sync")
	}
	gap := content[iSync : iSync+iCPU]
	// Before the fix, each hidden field still got FieldSeparator (\n\n), leaving long blank runs.
	if strings.Contains(gap, "\n\n\n\n\n") {
		t.Fatalf("excessive blank lines between Sync and CPU:\n%q", gap)
	}
}
