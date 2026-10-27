package lifecycle

import (
	"fmt"
	"strings"

	"github.com/perylemke/sbox/internal/usererr"
)

// TmuxPort is the HTTP port the tmux service uses when a sandbox is created with Running.
const TmuxPort = 8080

// ValidateAPI checks the API service settings of a create request.
func ValidateAPI(running bool, port int, argv []string) error {
	if len(argv) == 0 {
		return usererr.New(
			"api command is required",
			"pass the argv to run; the first item is the binary",
		)
	}
	for _, arg := range argv {
		if strings.TrimSpace(arg) == "" {
			return usererr.New(
				"api command has an empty argument",
				"omit empty items from the argv",
			)
		}
	}
	if port < 1 || port > 65535 {
		return usererr.New(
			fmt.Sprintf("invalid api port %d", port),
			"use a TCP port from 1 to 65535",
		)
	}
	if running && port == TmuxPort {
		return usererr.New(
			"api port 8080 is used by the tmux service",
			"choose another api port",
		)
	}
	return nil
}
