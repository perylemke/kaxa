package changelog

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var releaseDate = time.Date(2026, time.October, 29, 10, 0, 0, 0, time.UTC)

const (
	nextVersion  = "0.2.0"
	firstVersion = "0.1.0"
)

const header = `# Changelog

All notable changes to this project are documented in this file.

`

func TestRelease(t *testing.T) {
	tests := []struct {
		name    string
		doc     string
		version string
		want    string
	}{
		{
			name: "first release without compare links",
			doc: header + `## [Unreleased]

### Added

- Sandbox snapshots.

[Unreleased]: https://github.com/perylemke/kaxa/commits/main
`,
			version: "0.1.0",
			want: header + `## [Unreleased]

## [0.1.0] - 2026-10-29

### Added

- Sandbox snapshots.

[Unreleased]: https://github.com/perylemke/kaxa/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/perylemke/kaxa/releases/tag/v0.1.0
`,
		},
		{
			name: "next release links to the previous version",
			doc: header + `## [Unreleased]

### Fixed

- Modal timeout.

## [0.1.0] - 2026-10-01

### Added

- First release.

[Unreleased]: https://github.com/perylemke/kaxa/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/perylemke/kaxa/releases/tag/v0.1.0
`,
			version: "0.1.1",
			want: header + `## [Unreleased]

## [0.1.1] - 2026-10-29

### Fixed

- Modal timeout.

## [0.1.0] - 2026-10-01

### Added

- First release.

[Unreleased]: https://github.com/perylemke/kaxa/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/perylemke/kaxa/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/perylemke/kaxa/releases/tag/v0.1.0
`,
		},
		{
			name: "leading v is accepted",
			doc: header + `## [Unreleased]

- Something.
`,
			version: "v1.2.3",
			want: header + `## [Unreleased]

## [1.2.3] - 2026-10-29

- Something.
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Release(tt.doc, tt.version, releaseDate)
			if err != nil {
				t.Fatalf("Release() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Release() mismatch\n--- got ---\n%s\n--- want ---\n%s", got, tt.want)
			}
		})
	}
}

func TestReleaseErrors(t *testing.T) {
	withEntries := header + "## [Unreleased]\n\n- Something.\n"

	tests := []struct {
		name    string
		doc     string
		version string
		wantErr error
	}{
		{"pre-release version", withEntries, "0.1.0-rc.1", ErrInvalidVersion},
		{"not a version", withEntries, "latest", ErrInvalidVersion},
		{"empty version", withEntries, "", ErrInvalidVersion},
		{"missing unreleased section", header + "## [0.1.0] - 2026-10-01\n\n- x\n", nextVersion, ErrNoUnreleased},
		{"empty unreleased section", header + "## [Unreleased]\n\n## [0.1.0] - 2026-10-01\n\n- x\n", nextVersion, ErrEmptyUnreleased},
		{"only subsection headings", header + "## [Unreleased]\n\n### Added\n\n### Fixed\n", nextVersion, ErrEmptyUnreleased},
		{"version already released", header + "## [Unreleased]\n\n- y\n\n## [0.1.0] - 2026-10-01\n\n- x\n", firstVersion, ErrVersionExists},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Release(tt.doc, tt.version, releaseDate)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Release() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestNotes(t *testing.T) {
	doc := header + `## [Unreleased]

- Pending.

## [0.2.0] - 2026-10-29

### Added

- Feature B.

## [0.1.0] - 2026-10-01

### Added

- Feature A.

[Unreleased]: https://github.com/perylemke/kaxa/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/perylemke/kaxa/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/perylemke/kaxa/releases/tag/v0.1.0
`

	tests := []struct {
		name    string
		version string
		want    string
	}{
		{"middle section", "0.2.0", "### Added\n\n- Feature B.\n"},
		{"last section stops at the links", "v0.1.0", "### Added\n\n- Feature A.\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Notes(doc, tt.version)
			if err != nil {
				t.Fatalf("Notes() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Notes() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNotesErrors(t *testing.T) {
	doc := header + "## [Unreleased]\n\n- x\n\n## [0.1.0] - 2026-10-01\n\n## [0.0.9] - 2026-09-01\n\n- y\n"

	tests := []struct {
		name    string
		version string
		wantErr error
	}{
		{"missing version", "0.3.0", ErrVersionNotFound},
		{"section without content", "0.1.0", ErrVersionNotFound},
		{"unreleased is not a version", "Unreleased", ErrInvalidVersion},
		{"pre-release version", "0.1.0-rc.1", ErrInvalidVersion},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Notes(doc, tt.version)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Notes() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestNotesDoesNotMatchVersionPrefix(t *testing.T) {
	doc := header + "## [Unreleased]\n\n## [0.10.0] - 2026-10-29\n\n- ten\n"

	_, err := Notes(doc, "0.1.0")
	if !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("Notes() error = %v, want %v", err, ErrVersionNotFound)
	}
	if !strings.Contains(err.Error(), "0.1.0") {
		t.Errorf("error %q should name the version", err)
	}
}
