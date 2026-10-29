// Package reposync packs a local Git working tree for copy into a sandbox.
package reposync

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	// ErrNotRepository means start is not inside a Git working tree.
	ErrNotRepository = errors.New("not a git repository")
	// ErrGitDirFile means .git exists but is not a directory.
	ErrGitDirFile = errors.New("git metadata is not a directory")
	// ErrInvalidName means the repository directory name is not a single path segment.
	ErrInvalidName = errors.New("invalid repository name")
	// ErrPathEscape means a path leaves the repository root.
	ErrPathEscape = errors.New("path escapes repository")
	// ErrSecretFile means an untracked file looks like a credential and would be copied.
	ErrSecretFile = errors.New("untracked file looks like a secret")
)

// FindRoot walks up from start and returns the repository root and its directory name.
// The root must contain a .git directory. A .git file, used by linked worktrees, is rejected.
func FindRoot(start string) (string, string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", "", fmt.Errorf("resolve directory: %w", err)
	}
	for {
		info, err := os.Lstat(filepath.Join(dir, ".git"))
		if err == nil {
			if !info.IsDir() {
				return "", "", ErrGitDirFile
			}
			name := filepath.Base(dir)
			if err := validateRepoName(name); err != nil {
				return "", "", err
			}
			return dir, name, nil
		}
		if !os.IsNotExist(err) {
			return "", "", fmt.Errorf("check git metadata: %w", err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", ErrNotRepository
		}
		dir = parent
	}
}

func validateRepoName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) || strings.ContainsRune(name, 0) {
		return fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	return nil
}

func safeRel(rel string) (string, error) {
	if rel == "" || strings.ContainsRune(rel, 0) {
		return "", fmt.Errorf("%w: empty path", ErrPathEscape)
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("%w: %q", ErrPathEscape, rel)
	}
	cleaned := filepath.Clean(rel)
	parent := ".." + string(filepath.Separator)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, parent) {
		return "", fmt.Errorf("%w: %q", ErrPathEscape, rel)
	}
	return cleaned, nil
}
