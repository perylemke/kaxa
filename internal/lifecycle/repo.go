package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/perylemke/kaxa/internal/reposync"
	"github.com/perylemke/kaxa/internal/usererr"
)

// RepoCopy is one Git repository root to copy into the sandbox workspace.
type RepoCopy struct {
	Root string
	Repo string // directory name under workspace
}

// ResolveRepos resolves comma-separated paths to Git repository roots.
// When dirSet is false and raw is empty, it uses the current working directory (one repo).
// With dirSet true, raw is split on commas; empty segments are an error.
// Duplicate repository roots or duplicate workspace names are errors.
func ResolveRepos(raw string, dirSet bool) ([]RepoCopy, error) {
	if !dirSet {
		root, name, err := ResolveRepo(raw, false)
		if err != nil {
			return nil, err
		}
		return []RepoCopy{{Root: root, Repo: name}}, nil
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, usererr.New("directory is required", "pass one or more paths to git repositories, separated by commas")
	}
	parts := strings.Split(raw, ",")
	var out []RepoCopy
	seenRoot := make(map[string]struct{})
	seenName := make(map[string]struct{})
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, usererr.New("empty path in directory list", "remove extra commas or pass each path")
		}
		root, name, err := ResolveRepo(part, true)
		if err != nil {
			return nil, err
		}
		keyRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			keyRoot = root
		} else {
			root = keyRoot
		}
		if _, dup := seenRoot[keyRoot]; dup {
			return nil, usererr.New(
				fmt.Sprintf("duplicate repository path %q", part),
				"list each distinct git repository once",
			)
		}
		seenRoot[keyRoot] = struct{}{}
		if _, dup := seenName[name]; dup {
			return nil, usererr.New(
				fmt.Sprintf("two repositories would use the same workspace name %q", name),
				"rename one of the local directories so they differ",
			)
		}
		seenName[name] = struct{}{}
		out = append(out, RepoCopy{Root: root, Repo: name})
	}
	return out, nil
}

// NormalizeRepoList collapses Repos to Root/Repo when Repos is empty (MCP and tests),
// and keeps Root/Repo as the first copy when Repos is set.
func NormalizeRepoList(req *Request) {
	if req.Sync && len(req.Repos) == 0 && req.Root != "" {
		req.Repos = []RepoCopy{{Root: req.Root, Repo: req.Repo}}
	}
	if req.Sync && len(req.Repos) > 0 {
		req.Root = req.Repos[0].Root
		req.Repo = req.Repos[0].Repo
	}
}

// RepoCopyRoots returns distinct roots for API checks and project detection (first repo wins for API).
func RepoCopyRoots(req Request) []string {
	NormalizeRepoList(&req)
	var roots []string
	for _, r := range req.Repos {
		if !slices.Contains(roots, r.Root) {
			roots = append(roots, r.Root)
		}
	}
	return roots
}

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
				fmt.Sprintf("directory %q does not exist", dir),
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
		return usererr.Wrap(err, "current directory is not inside a git repository", "run kaxa create --sync from a git repository")
	case errors.Is(err, reposync.ErrGitDirFile):
		return usererr.Wrap(err, ".git is not a directory", "sync does not follow linked worktrees")
	case errors.Is(err, reposync.ErrInvalidName):
		return usererr.Wrap(err, "invalid repository directory name", "use a directory name that is a single path segment")
	default:
		return err
	}
}
