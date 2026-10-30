package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
)

// annotationFeature marks a command that only some providers implement. Its
// value is the sandbox.Feature a provider needs for the command to apply.
const annotationFeature = "kaxa_feature"

// providerTitles are the display names of providers in help. A provider
// without an entry is titled by its registry name.
var providerTitles = map[string]string{
	"sprites": "Sprites",
	"modal":   "Modal",
	"e2b":     "E2B",
}

func init() {
	cobra.AddTemplateFunc("commandSections", commandSections)
}

// featureCommand returns the annotations of a command that needs feature.
func featureCommand(feature sandbox.Feature) map[string]string {
	return map[string]string{annotationFeature: string(feature)}
}

type helpSection struct {
	title string
	cmds  []*cobra.Command
}

// commandSections renders the command lists of cmd's help, each preceded by a
// blank line. The root lists the commands every provider has under Commands and
// then one section per registered provider. Cobra keeps one group per command,
// so a command that several providers implement is listed in each section.
func commandSections(cmd *cobra.Command) string {
	sections := []helpSection{{title: "Available Commands:", cmds: availableCommands(cmd)}}
	if !cmd.HasParent() {
		sections = rootSections(cmd)
	}

	var b strings.Builder
	for _, section := range sections {
		if len(section.cmds) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n\n%s", section.title)
		for _, c := range section.cmds {
			fmt.Fprintf(&b, "\n  %-*s %s", c.NamePadding(), c.Name(), c.Short)
		}
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

func rootSections(root *cobra.Command) []helpSection {
	var general, perProvider []*cobra.Command
	for _, c := range availableCommands(root) {
		if c.Annotations[annotationFeature] == "" {
			general = append(general, c)
			continue
		}
		perProvider = append(perProvider, c)
	}
	sections := []helpSection{{title: "Commands:", cmds: general}}

	reg, ok := registryFromContext(root.Context())
	if !ok {
		// Without a registry no provider can be told apart, so keep the
		// commands visible rather than dropping them.
		return append(sections, helpSection{title: "Provider commands:", cmds: perProvider})
	}
	return append(sections, providerSections(reg, perProvider)...)
}

// providerSections lists, for each registered provider, the commands whose
// feature it implements. It reads capabilities, not credentials, so a command
// is listed before the user has configured a token.
func providerSections(reg *registry.Registry, cmds []*cobra.Command) []helpSection {
	var sections []helpSection
	for _, name := range reg.Names() {
		p, err := reg.Get(name)
		if err != nil {
			continue
		}
		caps := sandbox.CapabilitiesOf(p)
		var supported []*cobra.Command
		for _, c := range cmds {
			if caps.Has(sandbox.Feature(c.Annotations[annotationFeature])) {
				supported = append(supported, c)
			}
		}
		sections = append(sections, helpSection{title: providerTitle(name) + ":", cmds: supported})
	}
	return sections
}

func providerTitle(name string) string {
	if title, ok := providerTitles[name]; ok {
		return title
	}
	return name
}
