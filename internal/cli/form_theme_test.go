package cli

import (
	"io"
	"regexp"
	"strings"
	"testing"

	"charm.land/huh/v2"

	"github.com/perylemke/kaxa/internal/sandbox"
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

func TestCreateFormDaytonaNetworkBlockButtonsAlignLeft(t *testing.T) {
	t.Parallel()

	result := createFormResult{
		Provider:         formProviderDaytona,
		daytonaCatalogOK: true,
		daytonaTemplates: []sandbox.TemplateOption{{
			Name:  "daytona-small",
			Label: "daytona-small (1 CPU, 1 GB memory, 3 GB disk)",
		}},
	}
	apiRaw := createFormAPIRaw{}
	p := formDaytonaFake{formBareProvider{name: formProviderDaytona}}
	pages := buildCreateFormPages([]string{formProviderDaytona}, daytonaFormLookup(p), createFlags{}, &result, &apiRaw)
	grp := pages.options.WithTheme(huh.ThemeFunc(createWizardTheme)).WithWidth(80)
	_ = huh.NewForm(grp).WithTheme(huh.ThemeFunc(createWizardTheme)).WithWidth(80).UpdateFieldPositions()

	content := ansiEscape.ReplaceAllString(grp.Content(), "")
	titleCol := columnOf(content, "Block all network")
	yesCol := columnOf(content, "Yes")
	if titleCol < 0 || yesCol < 0 {
		t.Fatalf("content = %q", content)
	}
	// Button padding is two columns. A centered row sits near the middle of the description.
	if yesCol > titleCol+4 {
		t.Fatalf("Yes column %d, title column %d\n%s", yesCol, titleCol, content)
	}
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func columnOf(view, needle string) int {
	for line := range strings.SplitSeq(view, "\n") {
		if i := strings.Index(line, needle); i >= 0 {
			return i
		}
	}
	return -1
}
