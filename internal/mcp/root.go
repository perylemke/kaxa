package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/perylemke/kaxa/internal/usererr"
)

// EnvRoot names the directory that bounds which repositories MCP sync may copy.
// An empty value means the process working directory.
const EnvRoot = "KAXA_MCP_ROOT"

// RootFromEnv resolves KAXA_MCP_ROOT. An empty raw returns wd and does not
// reject a home directory; sync still refuses that copy later.
func RootFromEnv(raw, wd, home string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return wd, nil
	}
	return checkSyncRoot(raw, wd, home, "set KAXA_MCP_ROOT to a folder that holds the repositories, not your home directory or /")
}

// ClientRoot resolves the directory init and config store. dir empty means wd.
// The home directory and the filesystem root are refused.
func ClientRoot(dir, wd, home string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		dir = wd
	}
	return checkSyncRoot(dir, wd, home, "pass --root pointing at a folder that holds the repositories")
}

func checkSyncRoot(dir, wd, home, broadHint string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", usererr.New("sync root is empty", broadHint)
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(wd, dir)
	}
	info, err := os.Stat(dir) //gosec:disable G703 -- the operator sets this directory through KAXA_MCP_ROOT or --root; it is resolved and refused when it is home or /
	if err != nil {
		if os.IsNotExist(err) {
			return "", usererr.New(fmt.Sprintf("directory %s was not found", dir), broadHint)
		}
		return "", fmt.Errorf("sync root: %w", err)
	}
	if !info.IsDir() {
		return "", usererr.New(dir+" is not a directory", broadHint)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("sync root: %w", err)
	}
	if isBroadScope(resolved, home) {
		return "", usererr.New(
			fmt.Sprintf("the server working directory %s is too broad to bound which repositories sync may copy", resolved),
			broadHint,
		)
	}
	return resolved, nil
}
