package blaxel

import "strings"

// quoteArgv quotes each argument for a remote shell that receives one command
// string. The Blaxel process API has no argv. A single-quoted token cannot
// expand, and an embedded quote is closed, escaped, and reopened.
func quoteArgv(argv []string) string {
	parts := make([]string, len(argv))
	for i, arg := range argv {
		parts[i] = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
	}
	return strings.Join(parts, " ")
}
