package cli_test

import (
	"os"
	"strings"
	"testing"
)

// TestMain drops every KAXA_ variable the developer exported (for example by
// sourcing .env), so the tests do not depend on the local setup. Tests that need
// a variable set it with t.Setenv.
func TestMain(m *testing.M) {
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "KAXA_") {
			_ = os.Unsetenv(name)
		}
	}
	os.Exit(m.Run())
}
