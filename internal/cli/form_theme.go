package cli

import "charm.land/huh/v2"

func newThemedForm(groups ...*huh.Group) *huh.Form {
	return huh.NewForm(groups...).WithTheme(huh.ThemeFunc(huh.ThemeDracula))
}
