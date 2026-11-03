package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/mcp"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/usererr"
)

func TestMCPConfigDocument(t *testing.T) {
	t.Parallel()

	const home = "/Users/you"
	got, err := mcpConfigDocument(home, goosDarwin, "", "/Users/you/workspace")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	want := `{
  "kaxa": {
    "args": [
      "mcp",
      "start"
    ],
    "command": "kaxa",
    "env": {
      "HOME": "/Users/you",
      "KAXA_MCP_ROOT": "/Users/you/workspace",
      "PATH": "/Users/you:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin"
    },
    "logFile": "/Users/you/.kaxa/kaxa-mcp.log"
  }
}
`
	if string(got) != want {
		t.Fatalf("config =\n%s\nwant\n%s", got, want)
	}
}

func TestMCPConfigDocumentWindows(t *testing.T) {
	t.Parallel()

	const (
		home    = `C:\Users\you`
		appData = `C:\Users\you\AppData\Roaming`
	)
	got, err := mcpConfigDocument(home, goosWindows, appData, `C:\Users\you\workspace`)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	var doc map[string]mcpClientEntry
	if err := json.Unmarshal(got, &doc); err != nil {
		t.Fatalf("decode: %v\n%s", err, got)
	}
	entry := doc[mcpServerKey]
	if entry.Command != mcpCommandName || !slices.Equal(entry.Args, []string{commandMCP, "start"}) {
		t.Fatalf("entry = %+v", entry)
	}
	if entry.Env["HOME"] != home || entry.Env["APPDATA"] != appData || entry.Env[mcp.EnvRoot] != `C:\Users\you\workspace` {
		t.Fatalf("env = %v", entry.Env)
	}
	if entry.Env["PATH"] != home+mcpUnixPathExtra {
		t.Fatalf("PATH = %q", entry.Env["PATH"])
	}
	if entry.LogFile != appData+`\.kaxa\`+mcpLogFileName {
		t.Fatalf("logFile = %q", entry.LogFile)
	}
}

func TestMCPConfigDocumentRejectsUnsupportedOS(t *testing.T) {
	t.Parallel()

	_, err := mcpConfigDocument("/home/you", "freebsd", "", "/home/you/workspace")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported operating system") {
		t.Fatalf("error = %v", err)
	}
}

func TestMCPConfigDocumentRequiresAppDataOnWindows(t *testing.T) {
	t.Parallel()

	_, err := mcpConfigDocument(`C:\Users\you`, goosWindows, "", `C:\Users\you\workspace`)
	if err == nil || !strings.Contains(err.Error(), "APPDATA") {
		t.Fatalf("error = %v", err)
	}
}

func TestMCPAgentConfigPaths(t *testing.T) {
	t.Parallel()

	const (
		macHome = "/Users/you"
		linHome = "/home/you"
		winHome = `C:\Users\you`
		winApp  = `C:\Users\you\AppData\Roaming`
	)
	tests := []struct {
		agent   string
		goos    string
		home    string
		appData string
		want    []string
	}{
		{agentCursor, goosDarwin, macHome, "", []string{"/Users/you/.cursor/mcp.json"}},
		{agentCursor, goosWindows, winHome, winApp, []string{`C:\Users\you\.cursor\mcp.json`}},
		{agentGemini, goosLinux, linHome, "", []string{"/home/you/.gemini/settings.json"}},
		{"windsurf", goosDarwin, macHome, "", []string{"/Users/you/.codeium/windsurf/mcp_config.json"}},
		{"windsurf", goosWindows, winHome, winApp, []string{`C:\Users\you\AppData\Roaming\.codeium\windsurf\mcp_config.json`}},
		{agentClaude, goosDarwin, macHome, "", []string{
			"/Users/you/Library/Application Support/Claude/claude_desktop_config.json",
			"/Users/you/.claude.json",
		}},
		{agentClaude, goosLinux, linHome, "", []string{
			"/home/you/.config/Claude/claude_desktop_config.json",
			"/home/you/.claude.json",
		}},
		{agentClaude, goosWindows, winHome, winApp, []string{
			`C:\Users\you\AppData\Roaming\Claude\claude_desktop_config.json`,
			`C:\Users\you\.claude.json`,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.agent+"/"+tt.goos, func(t *testing.T) {
			t.Parallel()
			got, err := mcpAgentConfigPaths(tt.agent, tt.home, tt.goos, tt.appData)
			if err != nil {
				t.Fatalf("paths: %v", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("paths = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestMCPAgentConfigPathsRejectsUnknownAgent(t *testing.T) {
	t.Parallel()

	_, err := mcpAgentConfigPaths("codex", "/Users/you", goosDarwin, "")
	ue, ok := usererr.As(err)
	if !ok {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(ue.Message, "unknown agent") || !strings.Contains(ue.Hint, "claude, cursor, gemini, windsurf") {
		t.Fatalf("error = %+v", ue)
	}
}

func TestMCPConfigCommandPrintsEntry(t *testing.T) {
	home := t.TempDir()
	project := withMCPHost(t, home, goosDarwin, "")

	out := new(bytes.Buffer)
	cmd := NewRootCmd(registry.New())
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{commandMCP, commandConfig})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".kaxa")); !os.IsNotExist(err) {
		t.Fatalf("config created a log directory: %v", err)
	}
	var doc map[string]mcpClientEntry
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	entry := doc[mcpServerKey]
	wantRoot, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	if entry.LogFile != home+"/.kaxa/"+mcpLogFileName || entry.Env["HOME"] != home || entry.Env[mcp.EnvRoot] != wantRoot {
		t.Fatalf("entry = %+v", entry)
	}
}

func TestMCPConfigRootFlag(t *testing.T) {
	home := t.TempDir()
	withMCPHost(t, home, goosDarwin, "")
	root := filepath.Join(home, "other")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}

	out := new(bytes.Buffer)
	cmd := NewRootCmd(registry.New())
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{commandMCP, commandConfig, "--root", root})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var doc map[string]mcpClientEntry
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if doc[mcpServerKey].Env[mcp.EnvRoot] != want {
		t.Fatalf("root = %q", doc[mcpServerKey].Env[mcp.EnvRoot])
	}
}

func TestMCPConfigRefusesHome(t *testing.T) {
	home := t.TempDir()
	withMCPHost(t, home, goosDarwin, "")
	mcpCurrentWD = func() (string, error) { return home, nil }

	cmd := NewRootCmd(registry.New())
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{commandMCP, commandConfig})
	err := cmd.Execute()
	ue, ok := usererr.As(err)
	if !ok || !strings.Contains(ue.Message, "too broad") || !strings.Contains(ue.Hint, "--root") {
		t.Fatalf("err = %v", err)
	}
}

func TestMCPConfigSkipsProviderValidation(t *testing.T) {
	cmd := NewRootCmd(registry.New())
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{flagProvider, "nope", commandMCP, commandConfig})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config should not validate provider: %v", err)
	}
}

func TestMCPInitCursorMergesAndKeepsMode(t *testing.T) {
	home := t.TempDir()
	withMCPHost(t, home, runtime.GOOS, "")
	paths, err := mcpAgentConfigPaths(agentCursor, home, runtime.GOOS, "")
	if err != nil {
		t.Fatalf("paths: %v", err)
	}
	path := paths[0]
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	original := []byte("{\n  \"editor\": \"cursor\",\n  \"mcpServers\": {\n    \"other\": {\"command\": \"other\"},\n    \"kaxa\": {\"command\": \"old\"}\n  }\n}\n")
	if err := os.WriteFile(path, original, 0o644); err != nil { //gosec:disable G306 -- the test checks that init keeps this existing mode
		t.Fatalf("seed: %v", err)
	}

	out := new(bytes.Buffer)
	cmd := NewRootCmd(registry.New())
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{commandMCP, commandInit, agentCursor})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if strings.TrimSpace(out.String()) != path {
		t.Fatalf("stdout = %q, want %q", out.String(), path)
	}
	assertCursorMerge(t, path)
	if _, err := os.Stat(mcpJoinedLogDir(home)); !os.IsNotExist(err) {
		t.Fatalf("init created a log directory: %v", err)
	}
}

func TestMCPInitCreatesFileAndDirectory(t *testing.T) {
	home := t.TempDir()
	withMCPHost(t, home, runtime.GOOS, "")

	cmd := NewRootCmd(registry.New())
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{commandMCP, commandInit, agentGemini})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	paths, err := mcpAgentConfigPaths(agentGemini, home, runtime.GOOS, "")
	if err != nil {
		t.Fatalf("paths: %v", err)
	}
	info, err := os.Stat(paths[0])
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %o, want 600", info.Mode().Perm())
	}
	dir, err := os.Stat(filepath.Dir(paths[0]))
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if dir.Mode().Perm() != 0o755 {
		t.Fatalf("dir mode = %o, want 755", dir.Mode().Perm())
	}
	assertKaxaEntry(t, paths[0])
}

func TestMCPInitClaudeWritesBothFiles(t *testing.T) {
	home := t.TempDir()
	withMCPHost(t, home, runtime.GOOS, "")

	out := new(bytes.Buffer)
	cmd := NewRootCmd(registry.New())
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{commandMCP, commandInit, agentClaude})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	paths, err := mcpAgentConfigPaths(agentClaude, home, runtime.GOOS, "")
	if err != nil {
		t.Fatalf("paths: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("paths = %v", paths)
	}
	for _, path := range paths {
		if !strings.Contains(out.String(), path) {
			t.Fatalf("stdout = %q, missing %q", out.String(), path)
		}
		assertKaxaEntry(t, path)
	}
	first := readConfigFile(t, paths[0])
	second := readConfigFile(t, paths[1])
	if !bytes.Equal(kaxaEntryJSON(t, first), kaxaEntryJSON(t, second)) {
		t.Fatalf("claude entries differ:\n%s\n%s", first, second)
	}
}

func TestMCPInitLeavesInvalidJSONUntouched(t *testing.T) {
	home := t.TempDir()
	withMCPHost(t, home, runtime.GOOS, "")
	paths, err := mcpAgentConfigPaths(agentCursor, home, runtime.GOOS, "")
	if err != nil {
		t.Fatalf("paths: %v", err)
	}
	path := paths[0]
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	original := []byte("{")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cmd := NewRootCmd(registry.New())
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{commandMCP, commandInit, agentCursor})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error")
	}
	got := readConfigFile(t, path)
	if !bytes.Equal(got, original) {
		t.Fatalf("file changed to %q", got)
	}
}

func TestMCPInitRejectsUnknownAgent(t *testing.T) {
	home := t.TempDir()
	withMCPHost(t, home, runtime.GOOS, "")

	cmd := NewRootCmd(registry.New())
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{commandMCP, commandInit, "codex"})
	err := cmd.Execute()
	ue, ok := usererr.As(err)
	if !ok || !strings.Contains(ue.Hint, "windsurf") {
		t.Fatalf("error = %v", err)
	}
	entries, readErr := os.ReadDir(home)
	if readErr != nil {
		t.Fatalf("readdir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("init created %v", entries)
	}
}

func TestMCPInitRequiresAgent(t *testing.T) {
	cmd := NewRootCmd(registry.New())
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{commandMCP, commandInit})
	err := cmd.Execute()
	ue, ok := usererr.As(err)
	if !ok || !strings.Contains(ue.Message, "requires an agent") {
		t.Fatalf("error = %v", err)
	}
}

func withMCPHost(t *testing.T, home, goos, appData string) string {
	t.Helper()
	project := t.TempDir()
	prevGOOS, prevHome, prevApp, prevWD := mcpCurrentGOOS, mcpCurrentHome, mcpCurrentAppData, mcpCurrentWD
	mcpCurrentGOOS = func() string { return goos }
	mcpCurrentHome = func() (string, error) { return home, nil }
	mcpCurrentAppData = func() string { return appData }
	mcpCurrentWD = func() (string, error) { return project, nil }
	t.Cleanup(func() {
		mcpCurrentGOOS = prevGOOS
		mcpCurrentHome = prevHome
		mcpCurrentAppData = prevApp
		mcpCurrentWD = prevWD
	})
	return project
}

func assertCursorMerge(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %o, want 644", info.Mode().Perm())
	}
	assertKaxaEntry(t, path)
	var doc struct {
		Editor  string                     `json:"editor"`
		Servers map[string]json.RawMessage `json:"mcpServers"`
	}
	body := readConfigFile(t, path)
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("decode: %v\n%s", err, body)
	}
	if doc.Editor != agentCursor {
		t.Fatalf("editor = %q", doc.Editor)
	}
	var other struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(doc.Servers["other"], &other); err != nil {
		t.Fatalf("other: %v", err)
	}
	if other.Command != "other" {
		t.Fatalf("other = %+v", other)
	}
}

func assertKaxaEntry(t *testing.T, path string) {
	t.Helper()
	body := readConfigFile(t, path)
	var entry mcpClientEntry
	if err := json.Unmarshal(kaxaEntryJSON(t, body), &entry); err != nil {
		t.Fatalf("entry: %v", err)
	}
	if entry.Command != mcpCommandName || !slices.Equal(entry.Args, []string{commandMCP, "start"}) || entry.LogFile == "" {
		t.Fatalf("entry = %+v", entry)
	}
}

func readConfigFile(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path) //gosec:disable G304 -- path is a temp file created by this test
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return body
}

func kaxaEntryJSON(t *testing.T, body []byte) []byte {
	t.Helper()
	var doc struct {
		Servers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("decode: %v\n%s", err, body)
	}
	raw, ok := doc.Servers[mcpServerKey]
	if !ok {
		t.Fatalf("missing kaxa in %s", body)
	}
	return raw
}

func mcpJoinedLogDir(home string) string {
	return joinOS(runtime.GOOS, home, ".kaxa")
}
