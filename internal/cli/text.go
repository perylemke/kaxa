package cli

import (
	"strings"
	"unicode"
)

// plainText drops control characters, except line feeds, from text that came
// from a provider API or from the user. A sandbox label or an error body that
// carries an escape sequence could otherwise rewrite the terminal: hide text,
// move the cursor, or set the window title.
func plainText(s string) string {
	if !strings.ContainsFunc(s, isUnsafeRune) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isUnsafeRune(r) {
			return -1
		}
		return r
	}, s)
}

func isUnsafeRune(r rune) bool {
	return r != '\n' && (unicode.IsControl(r) || r == '\u2028' || r == '\u2029')
}
