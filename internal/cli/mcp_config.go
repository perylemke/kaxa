package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/mcp"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	mcpServerKey     = "kaxa"
	mcpCommandName   = "kaxa"
	mcpLogFileName   = "kaxa-mcp.log"
	mcpUnixPathExtra = ":/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin"
	commandMCP       = "mcp"
	flagProvider     = "--provider"
	commandInit      = "init"
	commandConfig    = "config"
	agentClaude      = "claude"
	agentCursor      = "cursor"
	agentGemini      = "gemini"
	agentWindsurf    = "windsurf"
)

// mcpClientEntry is the stdio server object kaxa mcp config prints and kaxa mcp init writes.
// Field order matches the JSON an agent pastes. The process does not open LogFile;
// a client that understands the field captures stderr there.
type mcpClientEntry struct {
	Args    []string          `json:"args"`
	Command string            `json:"command"`
	Env     map[string]string `json:"env"`
	LogFile string            `json:"logFile"`
}

// Hooks so tests can build the entry without the real home directory.
var (
	mcpCurrentGOOS    = func() string { return runtime.GOOS }
	mcpCurrentHome    = os.UserHomeDir
	mcpCurrentAppData = func() string { return os.Getenv("APPDATA") }
	mcpCurrentWD      = os.Getwd
)

func newMCPConfigCmd() *cobra.Command {
	var root string
	cmd := &cobra.Command{
		Use:   commandConfig,
		Short: "Print MCP client configuration",
		Long: `Print the kaxa MCP client entry as JSON.

The entry runs kaxa mcp start. It sets HOME, PATH, and KAXA_MCP_ROOT, and a logFile path. It does not include tokens, and kaxa does not open the log file. KAXA_MCP_ROOT bounds which repositories sync may copy. --root sets it; without the flag, config uses the current directory and refuses your home directory and /.`,
		Example: `  kaxa mcp config
  kaxa mcp config --root ~/workspace`,
		Args: noArgs("mcp config"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			doc, err := mcpConfigFromHost(root)
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(doc)
			return err
		},
	}
	cmd.Flags().StringVar(&root, "root", "", "directory that bounds which repositories sync may copy")
	return cmd
}

func mcpConfigFromHost(rootFlag string) ([]byte, error) {
	home, goos, appData, err := mcpHost()
	if err != nil {
		return nil, err
	}
	root, err := mcpSyncRoot(rootFlag, home)
	if err != nil {
		return nil, err
	}
	return mcpConfigDocument(home, goos, appData, root)
}

func mcpSyncRoot(flag, home string) (string, error) {
	flag = strings.TrimSpace(flag)
	var wd string
	if flag == "" || !filepath.IsAbs(flag) {
		var err error
		wd, err = mcpCurrentWD()
		if err != nil {
			if flag == "" {
				return "", usererr.Wrap(err, "working directory is unknown", "pass --root pointing at a folder that holds the repositories")
			}
			return "", fmt.Errorf("working directory: %w", err)
		}
	}
	return mcp.ClientRoot(flag, wd, home)
}

func mcpHost() (string, string, string, error) {
	home, err := mcpCurrentHome()
	if err != nil {
		return "", "", "", usererr.Wrap(err, "home directory is unknown", "set HOME and run the command again")
	}
	if home == "" {
		return "", "", "", usererr.New("home directory is unknown", "set HOME and run the command again")
	}
	return home, mcpCurrentGOOS(), mcpCurrentAppData(), nil
}

func mcpConfigDocument(home, goos, appData, root string) ([]byte, error) {
	entry, err := mcpClientEntryFor(home, goos, appData, root)
	if err != nil {
		return nil, err
	}
	out, err := json.MarshalIndent(map[string]mcpClientEntry{mcpServerKey: entry}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode mcp config: %w", err)
	}
	return append(out, '\n'), nil
}

func mcpClientEntryFor(home, goos, appData, root string) (mcpClientEntry, error) {
	logFile, err := mcpLogFile(home, goos, appData)
	if err != nil {
		return mcpClientEntry{}, err
	}
	env := map[string]string{
		"HOME":      home,
		"PATH":      home + mcpUnixPathExtra,
		mcp.EnvRoot: root,
	}
	if goos == goosWindows {
		env["APPDATA"] = appData
	}
	return mcpClientEntry{
		Args:    []string{commandMCP, "start"},
		Command: mcpCommandName,
		Env:     env,
		LogFile: logFile,
	}, nil
}

func mcpLogFile(home, goos, appData string) (string, error) {
	switch goos {
	case goosDarwin, goosLinux:
		return joinOS(goos, home, ".kaxa", mcpLogFileName), nil
	case goosWindows:
		if appData == "" {
			return "", usererr.New("could not resolve APPDATA", "set APPDATA and run the command again")
		}
		return joinOS(goos, appData, ".kaxa", mcpLogFileName), nil
	default:
		return "", unsupportedOS(goos)
	}
}

func unsupportedOS(goos string) error {
	return usererr.New(
		fmt.Sprintf("unsupported operating system %q", goos),
		"kaxa mcp supports darwin, linux, and windows",
	)
}

func joinOS(goos string, elem ...string) string {
	sep := "/"
	if goos == goosWindows {
		sep = `\`
	}
	return strings.Join(elem, sep)
}
