// Package changelog edits a Keep a Changelog file during a release: it turns
// the Unreleased section into a versioned one and extracts release notes.
package changelog

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	// ErrInvalidVersion means the version is not a final MAJOR.MINOR.PATCH.
	ErrInvalidVersion = errors.New("invalid version")
	// ErrNoUnreleased means the document has no "## [Unreleased]" heading.
	ErrNoUnreleased = errors.New("no [Unreleased] section")
	// ErrEmptyUnreleased means the Unreleased section has no entries.
	ErrEmptyUnreleased = errors.New("[Unreleased] section has no entries")
	// ErrVersionExists means the version already has a section.
	ErrVersionExists = errors.New("version already in changelog")
	// ErrVersionNotFound means the version has no section with entries.
	ErrVersionNotFound = errors.New("version not found in changelog")
)

const unreleasedHeading = "## [Unreleased]"

var (
	finalVersion = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`)
	linkDef      = regexp.MustCompile(`^\[[^\]]+\]: \S+`)
	unreleased   = regexp.MustCompile(`^\[Unreleased\]: (\S+?)/(?:compare/v(\S+?)\.\.\.HEAD|commits/\S+)$`)
)

// Release moves the entries under [Unreleased] into a new section for version,
// dated date, and updates the reference links at the end of the document.
func Release(doc, version string, date time.Time) (string, error) {
	version, err := normalize(version)
	if err != nil {
		return "", err
	}

	lines := strings.Split(doc, "\n")
	start := indexOfHeading(lines, unreleasedHeading)
	if start < 0 {
		return "", ErrNoUnreleased
	}
	if indexOfVersion(lines, version) >= 0 {
		return "", fmt.Errorf("%w: %s", ErrVersionExists, version)
	}

	end := sectionEnd(lines, start+1)
	body := trimBlank(lines[start+1 : end])
	if !hasEntries(body) {
		return "", ErrEmptyUnreleased
	}

	out := make([]string, 0, len(lines)+4)
	out = append(out, lines[:start+1]...)
	out = append(out, "", fmt.Sprintf("## [%s] - %s", version, date.Format(time.DateOnly)), "")
	out = append(out, body...)
	out = append(out, "")
	out = append(out, lines[end:]...)

	return strings.Join(updateLinks(out, version), "\n"), nil
}

// Notes returns the entries of the section for version, ready to be used as
// release notes.
func Notes(doc, version string) (string, error) {
	version, err := normalize(version)
	if err != nil {
		return "", err
	}

	lines := strings.Split(doc, "\n")
	start := indexOfVersion(lines, version)
	if start < 0 {
		return "", fmt.Errorf("%w: %s", ErrVersionNotFound, version)
	}

	body := trimBlank(lines[start+1 : sectionEnd(lines, start+1)])
	if !hasEntries(body) {
		return "", fmt.Errorf("%w: %s has no entries", ErrVersionNotFound, version)
	}

	return strings.Join(body, "\n") + "\n", nil
}

// normalize drops a leading v and accepts final versions only.
func normalize(version string) (string, error) {
	version = strings.TrimPrefix(version, "v")
	if !finalVersion.MatchString(version) {
		return "", fmt.Errorf("%w: %q (want MAJOR.MINOR.PATCH)", ErrInvalidVersion, version)
	}
	return version, nil
}

func indexOfHeading(lines []string, heading string) int {
	for i, line := range lines {
		if strings.TrimRight(line, " \t\r") == heading {
			return i
		}
	}
	return -1
}

func indexOfVersion(lines []string, version string) int {
	prefix := "## [" + version + "]"
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			return i
		}
	}
	return -1
}

// sectionEnd returns the index of the line that closes the section whose body
// starts at from: the next version heading or the first reference link.
func sectionEnd(lines []string, from int) int {
	for i := from; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## [") || linkDef.MatchString(lines[i]) {
			return i
		}
	}
	return len(lines)
}

func trimBlank(lines []string) []string {
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return lines[start:end]
}

// hasEntries reports whether the body has text besides blank lines and
// "###" subsection headings.
func hasEntries(body []string) bool {
	for _, line := range body {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "###") {
			return true
		}
	}
	return false
}

// updateLinks points [Unreleased] at the new version and adds the link of the
// new version. Documents without a recognized [Unreleased] link are unchanged.
func updateLinks(lines []string, version string) []string {
	for i, line := range lines {
		m := unreleased.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		base, previous := m[1], m[2]

		link := fmt.Sprintf("[%s]: %s/releases/tag/v%s", version, base, version)
		if previous != "" {
			link = fmt.Sprintf("[%s]: %s/compare/v%s...v%s", version, base, previous, version)
		}

		out := make([]string, 0, len(lines)+1)
		out = append(out, lines[:i]...)
		out = append(out, fmt.Sprintf("[Unreleased]: %s/compare/v%s...HEAD", base, version), link)
		out = append(out, lines[i+1:]...)
		return out
	}
	return lines
}
