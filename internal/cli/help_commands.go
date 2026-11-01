package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// annotationFeature marks a command that only some providers implement. Its
// value is the sandbox.Feature a provider needs for the command to apply.
const annotationFeature = "kaxa_feature"

func init() {
	cobra.AddTemplateFunc("commandSections", commandSections)
}

// featureCommand returns the annotations of a command that needs feature.
func featureCommand(feature sandbox.Feature) map[string]string {
	return map[string]string{annotationFeature: string(feature)}
}

// commandSections renders the command list of cmd's help, preceded by a blank
// line. The root lists every command under Commands, whichever providers
// implement it; a command group keeps Cobra's Available Commands title.
func commandSections(cmd *cobra.Command) string {
	cmds := availableCommands(cmd)
	if len(cmds) == 0 {
		return ""
	}
	title := "Available Commands:"
	if !cmd.HasParent() {
		title = "Commands:"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n\n%s", title)
	for _, c := range cmds {
		fmt.Fprintf(&b, "\n  %-*s %s", c.NamePadding(), c.Name(), c.Short)
	}
	return b.String()
}

func availableCommands(cmd *cobra.Command) []*cobra.Command {
	var cmds []*cobra.Command
	for _, c := range cmd.Commands() {
		if c.IsAvailableCommand() {
			cmds = append(cmds, c)
		}
	}
	return cmds
}
