package blaxel

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/perylemke/kaxa/internal/usererr"
)

// Blaxel names are DNS labels of at most 49 characters.
var dnsLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,47}[a-z0-9])?$`)

// imageRef is a container image reference without shell metacharacters.
var imageRef = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,200}$`)

var regionName = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func validateName(name string) error {
	if !dnsLabel.MatchString(name) {
		return usererr.New(
			fmt.Sprintf("invalid sandbox name %q", name),
			"use a DNS label of at most 49 characters: lowercase letters, digits, and hyphens",
		)
	}
	return nil
}

// validateTemplate accepts an empty name, which means the default image.
func validateTemplate(name string) error {
	if name == "" {
		return nil
	}
	if !imageRef.MatchString(name) || strings.Contains(name, "..") {
		return usererr.New(
			fmt.Sprintf("invalid template %q", name),
			"use an image reference: letters, digits, and . _ : / -",
		)
	}
	return nil
}

// validateRegion accepts an empty region, which means the default region.
func validateRegion(region string) error {
	if region == "" || regionName.MatchString(region) {
		return nil
	}
	return usererr.New(
		fmt.Sprintf("invalid Blaxel region %q", region),
		"use a region such as us-pdx-1",
	)
}

func validateMemory(mb int) error {
	if mb < 0 {
		return usererr.New(
			fmt.Sprintf("invalid Blaxel memory %d", mb),
			"set KAXA_BLAXEL_MEMORY to a whole number of megabytes, such as 4096",
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
