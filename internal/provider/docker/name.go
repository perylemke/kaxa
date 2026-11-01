package docker

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/perylemke/kaxa/internal/usererr"
)

// dnsLabel is the kaxa name of a sandbox. It fits the display name rule of the
// Sandboxes API (letters, digits, underscore, hyphen, at most 64 characters).
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

// imageRef matches a container image reference such as ubuntu:24.04 or
// registry.example.com/team/image@sha256:abc. The API has the last word on
// whether it exists and can be pulled.
var imageRef = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@-]{0,255}$`)

// validateTemplate accepts an empty name, which means the default image.
func validateTemplate(name string) error {
	if name == "" {
		return nil
	}
	if !imageRef.MatchString(name) || strings.Contains(name, "..") {
		return usererr.New(
			fmt.Sprintf("invalid template %q", name),
			"use an image reference: letters, digits, and . _ : / @ -",
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

// resourceID returns the last segment of a resource name such as sandboxes/abc.
func resourceID(name string) string {
	if _, after, found := strings.CutLast(name, "/"); found {
		return after
	}
	return name
}
