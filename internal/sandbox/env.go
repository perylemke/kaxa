package sandbox

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	// MaxEnvEntries is the most variables one sandbox environment accepts.
	MaxEnvEntries = 64
	// MaxEnvValue is the longest value, in bytes.
	MaxEnvValue = 8192

	envDir  = ".config/kaxa"
	envFile = "sandbox.env"
)

// EnvFilePath is the sandbox file that holds the user environment.
func EnvFilePath(home string) string {
	return home + "/" + envDir + "/" + envFile
}

// EnvDirPath is the directory that holds EnvFilePath.
func EnvDirPath(home string) string {
	return home + "/" + envDir
}

// ReservedEnvKey reports names the sandbox and the API service must not set.
// HOME, USER, PATH, and the KAXA_, MODAL_TOKEN_, FLY_, and SPRITES_ prefixes
// belong to the provider.
func ReservedEnvKey(key string) bool {
	switch key {
	case "HOME", "USER", "PATH":
		return true
	}
	for _, prefix := range []string{"KAXA_", "MODAL_TOKEN_", "FLY_", "SPRITES_"} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// CheckEnvValue rejects an overlong value, a comma, or a control character.
// kind is the noun in the error ("env" or "api env"). The error names key and
// never the value.
func CheckEnvValue(kind, key, value string) error {
	if len(value) > MaxEnvValue {
		return usererr.New(fmt.Sprintf("%s %s is too long", kind, key), "shorten the value")
	}
	if strings.Contains(value, ",") {
		return usererr.New(
			fmt.Sprintf("%s %s contains a comma", kind, key),
			"the provider joins variables with commas; avoid commas in the value",
		)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return usererr.New(fmt.Sprintf("%s %s contains a control character", kind, key), "remove it from the value")
		}
	}
	return nil
}

// ParseEnvFlag parses a comma-separated --env value. A bare KEY is rejected.
// The last entry for a key wins. An empty raw value returns nil.
func ParseEnvFlag(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	entries := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, usererr.New("env entry is empty", "use KEY=VALUE, separated by commas")
		}
		entries = append(entries, part)
	}
	return assignEnvEntries(entries)
}

// ParseEnvFile parses a dotenv file. Blank lines and full-line # comments are
// ignored. An export prefix and one layer of matching quotes are removed.
// There is no interpolation. The last entry for a key wins.
func ParseEnvFile(r io.Reader) ([]string, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), MaxEnvValue+1024)
	var entries []string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "export "); ok {
			line = strings.TrimSpace(rest)
		}
		entries = append(entries, line)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read env file: %w", err)
	}
	return assignEnvEntries(entries)
}

// ReadEnvFile reads a dotenv file at path. A missing path, a directory, and a
// symlink fail. The error names the path and never a value from the file.
func ReadEnvFile(path string) ([]string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, usererr.New(fmt.Sprintf("env file %s was not found", path), "pass a path to a file")
		}
		return nil, fmt.Errorf("env file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, usererr.New(fmt.Sprintf("env file %s is a symlink", path), "pass the real file")
	}
	if info.IsDir() {
		return nil, usererr.New(fmt.Sprintf("env file %s is a directory", path), "pass a path to a file")
	}
	if !info.Mode().IsRegular() {
		return nil, usererr.New(fmt.Sprintf("env file %s is not a regular file", path), "pass a path to a file")
	}
	f, err := openEnvFile(path)
	if err != nil {
		return nil, fmt.Errorf("env file: %w", err)
	}
	entries, err := ParseEnvFile(f)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return entries, err
}

// ResolveSandboxEnv merges a dotenv file and a comma-separated --env flag.
// Flag entries replace the same key from the file. An empty path skips the
// file. An empty flag adds nothing.
func ResolveSandboxEnv(path, flag string) ([]string, error) {
	var file []string
	if strings.TrimSpace(path) != "" {
		var err error
		file, err = ReadEnvFile(path)
		if err != nil {
			return nil, err
		}
	}
	overlay, err := ParseEnvFlag(flag)
	if err != nil {
		return nil, err
	}
	merged := MergeEnv(file, overlay)
	if len(merged) > MaxEnvEntries {
		return nil, usererr.New(
			fmt.Sprintf("too many env entries (maximum %d)", MaxEnvEntries),
			"pass fewer variables",
		)
	}
	return merged, nil
}

// MergeEnv overlays KEY=VALUE layers. A later layer replaces the same key.
// The result keeps the order of first appearance. Empty input returns nil.
func MergeEnv(layers ...[]string) []string {
	index := make(map[string]int)
	var out []string
	for _, layer := range layers {
		for _, entry := range layer {
			key, _, ok := strings.Cut(entry, "=")
			if !ok || key == "" {
				continue
			}
			if i, exists := index[key]; exists {
				out[i] = entry
				continue
			}
			index[key] = len(out)
			out = append(out, entry)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// EnvMap converts KEY=VALUE entries into a map. The last entry for a key wins.
func EnvMap(entries []string) map[string]string {
	if len(entries) == 0 {
		return nil
	}
	out := make(map[string]string, len(entries))
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// FormatEnvFile renders KEY=VALUE lines for the sandbox env file.
func FormatEnvFile(entries []string) []byte {
	var b strings.Builder
	for _, entry := range entries {
		b.WriteString(entry)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

// EnvFromMap converts a provider environment map into KEY=VALUE entries.
// Reserved and invalid keys are left out so the toolchain PATH stays put.
// Keys are sorted so the result is stable.
func EnvFromMap(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		if !ValidEnvKey(key) || ReservedEnvKey(key) {
			continue
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil
	}
	slices.Sort(keys)
	out := make([]string, len(keys))
	for i, key := range keys {
		out[i] = key + "=" + env[key]
	}
	return out
}

func assignEnvEntries(entries []string) ([]string, error) {
	if len(entries) > MaxEnvEntries {
		return nil, usererr.New(
			fmt.Sprintf("too many env entries (maximum %d)", MaxEnvEntries),
			"pass fewer variables",
		)
	}
	order := make([]string, 0, len(entries))
	vals := make(map[string]string, len(entries))
	for _, entry := range entries {
		key, value, hasValue := strings.Cut(entry, "=")
		key = strings.TrimSpace(key)
		if !ValidEnvKey(key) {
			return nil, usererr.New(
				"invalid env key",
				"use KEY=VALUE, with letters, digits, and underscores, not starting with a digit; a value cannot contain a comma",
			)
		}
		if !hasValue {
			return nil, usererr.New(
				fmt.Sprintf("env %s needs a value", key),
				"pass KEY=VALUE, or use --env-file; a value cannot contain a comma",
			)
		}
		if ReservedEnvKey(key) {
			return nil, usererr.New(fmt.Sprintf("env %s is reserved", key), "choose another variable name")
		}
		value = unquoteEnv(value)
		if err := CheckEnvValue("env", key, value); err != nil {
			return nil, err
		}
		if _, seen := vals[key]; !seen {
			order = append(order, key)
		}
		vals[key] = value
	}
	if len(order) == 0 {
		return nil, nil
	}
	out := make([]string, len(order))
	for i, key := range order {
		out[i] = key + "=" + vals[key]
	}
	return out, nil
}

func unquoteEnv(value string) string {
	if len(value) >= 2 {
		if value[0] == '"' && value[len(value)-1] == '"' {
			return value[1 : len(value)-1]
		}
		if value[0] == '\'' && value[len(value)-1] == '\'' {
			return value[1 : len(value)-1]
		}
	}
	return value
}
