package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

// resolveSandboxEnv merges env_file with names copied from the server.
// A name replaces the same key from the file. The result is KEY=VALUE and is
// not logged.
func (s *service) resolveSandboxEnv(in createIn) ([]string, error) {
	var file []string
	if strings.TrimSpace(in.EnvFile) != "" {
		path, err := s.envFilePath(in.EnvFile)
		if err != nil {
			return nil, err
		}
		file, err = sandbox.ReadEnvFile(path)
		if err != nil {
			return nil, err
		}
	}
	named, err := s.envFromAllowlist(in.Env)
	if err != nil {
		return nil, err
	}
	merged := sandbox.MergeEnv(file, named)
	if len(merged) > sandbox.MaxEnvEntries {
		return nil, usererr.New(
			fmt.Sprintf("too many env entries (maximum %d)", sandbox.MaxEnvEntries),
			"pass fewer variables",
		)
	}
	return merged, nil
}

// envFromAllowlist copies named server variables. KEY=VALUE is rejected.
// An empty allowlist rejects every name.
func (s *service) envFromAllowlist(names []string) ([]string, error) {
	var out []string
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if key, _, ok := strings.Cut(name, "="); ok {
			return nil, usererr.New(
				fmt.Sprintf("env %s cannot include a value", key),
				"pass the variable name and list it in "+EnvSandboxAllow+", or pass env_file",
			)
		}
		if !sandbox.ValidEnvKey(name) {
			return nil, usererr.New("invalid env key "+name, "use a name of letters, digits and underscores")
		}
		if sandbox.ReservedEnvKey(name) {
			return nil, usererr.New(fmt.Sprintf("env %s is reserved", name), "pick another name")
		}
		if _, ok := s.sandboxEnv[name]; !ok {
			return nil, usererr.New(
				fmt.Sprintf("env %s cannot be copied from the server environment; the user must list it in %s", name, EnvSandboxAllow),
				"pass env_file for other variables",
			)
		}
		value, ok := s.hostEnv(name)
		if !ok {
			return nil, usererr.New(fmt.Sprintf("env %s is not set", name), "export it in the server environment")
		}
		if err := sandbox.CheckEnvValue("env", name, value); err != nil {
			return nil, err
		}
		out = append(out, name+"="+value)
	}
	return out, nil
}

// envFilePath resolves a dotenv path and keeps it inside the server root.
// A symlink is followed, and the target must stay inside that root.
func (s *service) envFilePath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	path := raw
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.root, path)
	}
	path = filepath.Clean(path)
	base := filepath.Clean(s.root)
	root, err := filepath.EvalSymlinks(s.root)
	if err != nil {
		return "", fmt.Errorf("resolve server root: %w", err)
	}
	if !pathInside(path, base) && !pathInside(path, root) {
		return "", outsideEnvFile()
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", usererr.New(fmt.Sprintf("env file %s was not found", raw), "pass a file inside the server root")
	}
	if info.IsDir() {
		return "", usererr.New(fmt.Sprintf("env file %s is a directory", raw), "pass a file")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", usererr.New(fmt.Sprintf("env file %s was not found", raw), "pass a file inside the server root")
	}
	if !pathInside(resolved, root) {
		return "", outsideEnvFile()
	}
	info, err = os.Lstat(resolved)
	if err != nil {
		return "", usererr.New(fmt.Sprintf("env file %s was not found", raw), "pass a file inside the server root")
	}
	if info.IsDir() {
		return "", usererr.New(fmt.Sprintf("env file %s is a directory", raw), "pass a file")
	}
	return resolved, nil
}

func outsideEnvFile() error {
	return usererr.New("env file is outside the server root", "pass a file inside the server working directory")
}

func pathInside(path, root string) bool {
	path, root = filepath.Clean(path), filepath.Clean(root)
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func sandboxEnvKeys(entries []string) []string {
	if len(entries) == 0 {
		return nil
	}
	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		key, _, ok := strings.Cut(entry, "=")
		if ok && key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}
