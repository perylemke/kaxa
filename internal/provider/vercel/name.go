package vercel

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/perylemke/kaxa/internal/usererr"
)

var sandboxName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

func validateName(name string) error {
	if !sandboxName.MatchString(name) {
		return usererr.New(
			fmt.Sprintf("invalid sandbox name %q", name),
			"use lowercase letters, digits, hyphens, and underscores, starting with a letter or digit",
		)
	}
	return nil
}

// imageName matches image references such as vercel/sandbox/universal or
// team/project/repo@sha256:... The API has the last word on whether one exists.
var imageName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@-]{0,255}$`)

// validateTemplate accepts an empty name, which means the default image.
func validateTemplate(name string) error {
	if name == "" {
		return nil
	}
	if !imageName.MatchString(name) || strings.Contains(name, "..") {
		return usererr.New(
			fmt.Sprintf("invalid template %q", name),
			"use an image reference: letters, digits, and . _ : / @ -",
		)
	}
	return nil
}

// vcpusFor converts a CPU request into whole vCPUs. Vercel accepts 1 or an even number.
func vcpusFor(cpu float64) (int, error) {
	if cpu == 0 {
		return 0, nil
	}
	n := int(cpu)
	if cpu != math.Trunc(cpu) || n < 1 || (n != 1 && n%2 != 0) {
		return 0, usererr.New(
			fmt.Sprintf("invalid cpu %v for vercel", cpu),
			"use 1 or an even number of CPUs, such as 2 or 4",
		)
	}
	return n, nil
}

func validateRepoDir(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0) {
		return usererr.New(fmt.Sprintf("invalid repository directory %q", name), "use a single path segment")
	}
	return nil
}
