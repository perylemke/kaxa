package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/usererr"
)

const mcpConfigFileMode os.FileMode = 0o600

func newMCPInitCmd() *cobra.Command {
	var root string
	cmd := &cobra.Command{
		Use:   "init <agent>",
		Short: "Install the MCP server into an agent",
		Long: `Write the kaxa MCP client entry into an agent's user config.

Supported agents are claude, cursor, gemini, and windsurf. claude updates Claude Desktop and Claude Code with the same entry. The entry sets KAXA_MCP_ROOT and does not include tokens. --root sets that directory; without the flag, init uses the current directory and refuses your home directory and /. Other servers in the file are left in place.`,
		Example: `  kaxa mcp init cursor --root ~/workspace
  kaxa mcp init claude`,
		Args: mcpInitArgs,
		RunE: runMCPInit,
	}
	cmd.Flags().StringVar(&root, "root", "", "directory that bounds which repositories sync may copy")
	return cmd
}

func mcpInitArgs(_ *cobra.Command, args []string) error {
	if len(args) != 1 {
		return usererr.New(
			"mcp init requires an agent name",
			"kaxa mcp init ["+strings.Join(mcpAgentNames, "|")+"]",
		)
	}
	return nil
}

func runMCPInit(cmd *cobra.Command, args []string) error {
	home, goos, appData, err := mcpHost()
	if err != nil {
		return err
	}
	root, err := mcpSyncRoot(cmd.Flags().Lookup("root").Value.String(), home)
	if err != nil {
		return err
	}
	paths, err := mcpAgentConfigPaths(args[0], home, goos, appData)
	if err != nil {
		return err
	}
	entry, err := mcpClientEntryFor(home, goos, appData, root)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if err := writeMCPConfig(path, entry); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), path); err != nil {
			return err
		}
	}
	return nil
}

var mcpAgentNames = []string{agentClaude, agentCursor, agentGemini, agentWindsurf}

func mcpAgentConfigPaths(agent, home, goos, appData string) ([]string, error) {
	switch agent {
	case agentCursor:
		return []string{joinOS(goos, home, ".cursor", "mcp.json")}, nil
	case agentGemini:
		return []string{joinOS(goos, home, ".gemini", "settings.json")}, nil
	case agentWindsurf:
		return windsurfConfigPath(home, goos, appData)
	case agentClaude:
		desktop, err := claudeDesktopConfigPath(home, goos, appData)
		if err != nil {
			return nil, err
		}
		return []string{desktop, joinOS(goos, home, ".claude.json")}, nil
	default:
		return nil, usererr.New(
			fmt.Sprintf("unknown agent %q", agent),
			"supported: "+strings.Join(mcpAgentNames, ", "),
		)
	}
}

func windsurfConfigPath(home, goos, appData string) ([]string, error) {
	switch goos {
	case goosDarwin, goosLinux:
		return []string{joinOS(goos, home, ".codeium", "windsurf", "mcp_config.json")}, nil
	case goosWindows:
		if appData == "" {
			return nil, missingAppData()
		}
		return []string{joinOS(goos, appData, ".codeium", "windsurf", "mcp_config.json")}, nil
	default:
		return nil, unsupportedOS(goos)
	}
}

func claudeDesktopConfigPath(home, goos, appData string) (string, error) {
	switch goos {
	case goosDarwin:
		return joinOS(goos, home, "Library", "Application Support", "Claude", "claude_desktop_config.json"), nil
	case goosLinux:
		return joinOS(goos, home, ".config", "Claude", "claude_desktop_config.json"), nil
	case goosWindows:
		if appData == "" {
			return "", missingAppData()
		}
		return joinOS(goos, appData, "Claude", "claude_desktop_config.json"), nil
	default:
		return "", unsupportedOS(goos)
	}
}

func missingAppData() error {
	return usererr.New("could not resolve APPDATA", "set APPDATA and run the command again")
}

func writeMCPConfig(path string, entry mcpClientEntry) error {
	existing, err := os.ReadFile(path) //gosec:disable G304 -- path is an agent config under the home directory, chosen from a fixed list
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read agent config: %w", err)
	}
	if os.IsNotExist(err) {
		existing = nil
	}
	merged, err := mergeMCPConfig(existing, entry)
	if err != nil {
		return err
	}
	// Agent config directories match the mode desktop apps already use.
	// The file itself stays 0600. An existing file keeps its mode: WriteFile
	// does not change permissions when the file is already there.
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil { //gosec:disable G301 -- desktop agent config directories are mode 0755; the config file is 0600
		return fmt.Errorf("create agent config directory: %w", err)
	}
	if err := os.WriteFile(path, merged, mcpConfigFileMode); err != nil { //gosec:disable G304,G703 -- path is an agent config under the home directory, chosen from a fixed list
		return fmt.Errorf("write agent config: %w", err)
	}
	return nil
}

func mergeMCPConfig(existing []byte, entry mcpClientEntry) ([]byte, error) {
	root := map[string]any{}
	if len(bytes.TrimSpace(existing)) > 0 {
		if err := json.Unmarshal(existing, &root); err != nil {
			return nil, usererr.Wrap(err, "agent config is not valid JSON", "fix the file or move it aside, then run kaxa mcp init again")
		}
	}
	servers, err := mcpServersObject(root)
	if err != nil {
		return nil, err
	}
	servers[mcpServerKey] = entry
	root["mcpServers"] = servers
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode agent config: %w", err)
	}
	return append(out, '\n'), nil
}

func mcpServersObject(root map[string]any) (map[string]any, error) {
	raw, ok := root["mcpServers"]
	if !ok || raw == nil {
		return map[string]any{}, nil
	}
	servers, ok := raw.(map[string]any)
	if !ok {
		return nil, usererr.New(
			"mcpServers must be an object",
			"edit the agent config so mcpServers is a JSON object",
		)
	}
	return servers, nil
}

func dirOf(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[:i]
	}
	return "."
}
