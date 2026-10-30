package cli

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// annotationFlagScope marks a flag that only some providers honor. Its value
// is flagScopeProvider.
const (
	annotationFlagScope = "kaxa_flag_scope"
	flagScopeProvider   = "provider"
)

func init() {
	cobra.AddTemplateFunc("flagSections", flagSections)
}

// markProviderFlags tags the named flags of cmd as provider-specific, so help
// lists them apart from the flags every provider accepts. The names are
// literals at each call site, so an unknown one is a programming error.
func markProviderFlags(cmd *cobra.Command, names ...string) {
	for _, name := range names {
		if err := cmd.Flags().SetAnnotation(name, annotationFlagScope, []string{flagScopeProvider}); err != nil {
			panic(fmt.Sprintf("mark provider flag: %v", err))
		}
	}
}

// flagSections renders the local flags of cmd as Flags and Provider flags, each
// preceded by a blank line. An empty section is left out.
func flagSections(cmd *cobra.Command) string {
	local := cmd.LocalFlags()
	generic := pflag.NewFlagSet(cmd.Name(), pflag.ContinueOnError)
	provider := pflag.NewFlagSet(cmd.Name(), pflag.ContinueOnError)
	generic.SortFlags = local.SortFlags
	provider.SortFlags = local.SortFlags

	local.VisitAll(func(f *pflag.Flag) {
		if isProviderFlag(f) {
			provider.AddFlag(f)
			return
		}
		generic.AddFlag(f)
	})

	var b strings.Builder
	for _, section := range []struct {
		title string
		flags *pflag.FlagSet
	}{{"Flags:", generic}, {"Provider flags:", provider}} {
		if usages := strings.TrimRightFunc(section.flags.FlagUsages(), unicode.IsSpace); usages != "" {
			fmt.Fprintf(&b, "\n\n%s\n%s", section.title, usages)
		}
	}
	return b.String()
}

func isProviderFlag(f *pflag.Flag) bool {
	return slices.Contains(f.Annotations[annotationFlagScope], flagScopeProvider)
}
