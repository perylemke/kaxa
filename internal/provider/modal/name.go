package modal

import (
	"fmt"
	"regexp"

	"github.com/perylemke/kaxa/internal/usererr"
)

var dnsLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func validateName(name string) error {
	if !dnsLabel.MatchString(name) {
		return usererr.New(
			fmt.Sprintf("invalid sandbox name %q", name),
			"use a DNS label: lowercase letters, digits, and hyphens",
		)
	}
	return nil
}
