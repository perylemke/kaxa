package daytona

import (
	"fmt"
	"regexp"
	"strings"

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

// imageName matches the names Daytona gives images and the img_ IDs. The API
// has the last word on whether one exists.
var imageName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// validateTemplate accepts an empty name, which means the default image.
func validateTemplate(name string) error {
	if name == "" {
		return nil
	}
	if !imageName.MatchString(name) || strings.Contains(name, "..") {
		return usererr.New(
			fmt.Sprintf("invalid template %q", name),
			"use the Daytona image name or its img_ ID: letters, digits, and . _ -",
		)
	}
	return nil
}

func validateRepoDir(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0) {
		return usererr.New(fmt.Sprintf("invalid repository directory %q", name), "use a single path segment")
	}
	return nil
}
