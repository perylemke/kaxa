package sprites

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/perylemke/kaxa/internal/usererr"
)

var dnsLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

const nameHint = "use a DNS label: lowercase letters, digits, and hyphens"

func validateName(name string) error {
	if name == "" || name == "." || name == ".." {
		return usererr.New(fmt.Sprintf("invalid sandbox name %q", name), nameHint)
	}
	if strings.Contains(name, "/") {
		return usererr.New(fmt.Sprintf("invalid sandbox name %q", name), nameHint)
	}
	if !dnsLabel.MatchString(name) {
		return usererr.New(fmt.Sprintf("invalid sandbox name %q", name), nameHint)
	}
	return nil
}
