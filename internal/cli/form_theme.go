package cli

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// createWizardTheme is Dracula with no between-field gap; visible wrapper fields
// add spacing so skipped (hidden) slots do not leave blank runs.
func createWizardTheme(isDark bool) *huh.Styles {
	t := huh.ThemeDracula(isDark)
	t.FieldSeparator = lipgloss.NewStyle()
	return t
}

func newThemedForm(groups ...*huh.Group) *huh.Form {
	return huh.NewForm(groups...).WithTheme(huh.ThemeFunc(huh.ThemeDracula))
}

func newCreateWizardForm(groups ...*huh.Group) *huh.Form {
	return huh.NewForm(groups...).
		WithTheme(huh.ThemeFunc(createWizardTheme)).
		WithViewHook(func(v tea.View) tea.View {
			v.ReportFocus = true
			v.AltScreen = true
			return v
		})
}
