package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/perylemke/sbox/internal/reposync"
	"github.com/perylemke/sbox/internal/usererr"
)

// ResolveRepo finds the Git repository to copy. It returns the repository
// root and its directory name. When dirSet is false it starts from the current
// directory. A subdirectory resolves to the whole repository.
func ResolveRepo(dir string, dirSet bool) (string, string, error) {
	start, explicit, err := syncStart(dir, dirSet)
	if err != nil {
		return "", "", err
	}
	root, name, err := reposync.FindRoot(start)
	if err != nil {
		return "", "", syncRepoError(err, explicit)
	}
	return root, name, nil
}

func syncStart(dir string, dirSet bool) (string, string, error) {
	if !dirSet {
		wd, err := os.Getwd()
		if err != nil {
			return "", "", fmt.Errorf("working directory: %w", err)
		}
		return wd, "", nil
	}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", "", usererr.New("directory is required", "pass a path to a git repository")
	}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", usererr.New(
				fmt.Sprintf("directory %q was not found", dir),
				"pass an existing directory",
			)
		}
		return "", "", fmt.Errorf("stat directory: %w", err)
	}
	if !info.IsDir() {
		return "", "", usererr.New(
			fmt.Sprintf("%q is not a directory", dir),
			"pass a directory inside a git repository",
		)
	}
	return dir, dir, nil
}

func syncRepoError(err error, dir string) error {
	switch {
	case errors.Is(err, reposync.ErrNotRepository):
		if dir != "" {
			return usererr.Wrap(
				err,
				dir+" is not inside a git repository",
				"pass a directory inside a git repository",
			)
		}
		return usererr.Wrap(err, "current directory is not inside a git repository", "run sbox create --sync from a git repository")
	case errors.Is(err, reposync.ErrGitDirFile):
		return usererr.Wrap(err, ".git is not a directory", "sync does not follow linked worktrees")
	case errors.Is(err, reposync.ErrInvalidName):
		return usererr.Wrap(err, "invalid repository directory name", "use a directory name that is a single path segment")
	default:
		return err
	}
}
