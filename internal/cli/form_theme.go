package cli

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

func newThemedForm(groups ...*huh.Group) *huh.Form {
	return huh.NewForm(groups...).WithTheme(huh.ThemeFunc(huh.ThemeDracula))
}

func newCreateWizardForm(groups ...*huh.Group) *huh.Form {
	return huh.NewForm(groups...).
		WithTheme(huh.ThemeFunc(huh.ThemeDracula)).
		WithViewHook(func(v tea.View) tea.View {
			v.ReportFocus = true
			v.AltScreen = true
			return v
		})
}
