package sprites

import (
	"fmt"
	"regexp"
	"strings"
)

var dnsLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func validateName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid sandbox name %q", name)
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("invalid sandbox name %q", name)
	}
	if !dnsLabel.MatchString(name) {
		return fmt.Errorf("invalid sandbox name %q", name)
	}
	return nil
}
