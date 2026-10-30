package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	cobra.AddTemplateFunc("subcommandAliases", subcommandAliases)
}

func subcommandAliases(cmd *cobra.Command) string {
	type alias struct {
		name    string
		command string
	}
	var aliases []alias
	width := 0
	for _, child := range cmd.Commands() {
		if !child.IsAvailableCommand() {
			continue
		}
		for _, name := range child.Aliases {
			aliases = append(aliases, alias{name: name, command: child.Name()})
			if len(name) > width {
				width = len(name)
			}
		}
	}
	if len(aliases) == 0 {
		return ""
	}
	sort.Slice(aliases, func(i, j int) bool {
		return aliases[i].name < aliases[j].name
	})

	var b strings.Builder
	b.WriteString("\n\nAliases:")
	for _, alias := range aliases {
		fmt.Fprintf(&b, "\n  %-*s %s", width, alias.name, alias.command)
	}
	return b.String()
}

const usageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{commandSections .}}{{subcommandAliases .}}{{end}}{{if .HasAvailableLocalFlags}}{{flagSections .}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
