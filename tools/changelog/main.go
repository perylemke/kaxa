// Command changelog edits CHANGELOG.md during a release. It is a development
// tool and is not part of the kaxa binary.
//
//	go run ./tools/changelog release <version>   move [Unreleased] into <version>
//	go run ./tools/changelog notes <version>     print the notes of <version>
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/perylemke/kaxa/internal/changelog"
)

const changelogFile = "CHANGELOG.md"

func main() {
	if err := run(os.Args[1:], changelogFile, time.Now(), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "changelog:", err)
		os.Exit(1)
	}
}

func run(args []string, path string, now time.Time, stdout io.Writer) error {
	if len(args) != 2 {
		return errors.New("usage: changelog release|notes <version>")
	}
	command, version := args[0], args[1]

	switch command {
	case "release":
		return release(path, version, now)
	case "notes":
		return notes(path, version, stdout)
	default:
		return fmt.Errorf("unknown command %q (want release or notes)", command)
	}
}

func release(path, version string, now time.Time) error {
	doc, err := read(path)
	if err != nil {
		return err
	}
	updated, err := changelog.Release(doc, version, now)
	if err != nil {
		return err
	}
	// WriteFile keeps the mode of an existing file; 0o600 only applies to a new one.
	//gosec:disable G703 -- fixed file name, not user input
	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func notes(path, version string, stdout io.Writer) error {
	doc, err := read(path)
	if err != nil {
		return err
	}
	text, err := changelog.Notes(doc, version)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, text)
	return err
}

func read(path string) (string, error) {
	data, err := os.ReadFile(path) //gosec:disable G304 -- fixed file name, not user input
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}
