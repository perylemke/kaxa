package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	cmdRelease = "release"
	cmdNotes   = "notes"
	version    = "0.1.0"
)

const doc = `# Changelog

## [Unreleased]

- Something.
`

var now = time.Date(2026, time.October, 29, 10, 0, 0, 0, time.UTC)

func writeDoc(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "CHANGELOG.md")
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunRelease(t *testing.T) {
	path := writeDoc(t)

	if err := run([]string{cmdRelease, version}, path, now, &bytes.Buffer{}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	got, err := os.ReadFile(path) //nolint:gosec // path is a test temp file
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "## ["+version+"] - 2026-10-29") {
		t.Errorf("changelog not updated:\n%s", got)
	}
}

func TestRunNotes(t *testing.T) {
	path := writeDoc(t)
	if err := run([]string{cmdRelease, version}, path, now, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := run([]string{cmdNotes, "v" + version}, path, now, &out); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if out.String() != "- Something.\n" {
		t.Errorf("notes = %q", out.String())
	}
}

func TestRunErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"no arguments", nil},
		{"unknown command", []string{"publish", version}},
		{"missing version", []string{cmdRelease}},
		{"extra argument", []string{cmdNotes, version, "extra"}},
		{"invalid version", []string{cmdRelease, version + "-rc.1"}},
		{"notes for missing version", []string{cmdNotes, "9.9.9"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeDoc(t)
			if err := run(tt.args, path, now, &bytes.Buffer{}); err == nil {
				t.Fatal("run() error = nil, want error")
			}
		})
	}
}

func TestRunMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "CHANGELOG.md")
	if err := run([]string{cmdNotes, version}, missing, now, &bytes.Buffer{}); err == nil {
		t.Fatal("run() error = nil, want error")
	}
}
