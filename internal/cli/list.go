package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/registry"
)

func newListCmd(reg *registry.Registry) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List sandboxes of all providers, or of --provider",
		Args:    noArgs("list"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			providers, names, err := listTargets(cmd, reg)
			if err != nil {
				return err
			}
			infos, err := collectSandboxes(cmd.Context(), cmd.ErrOrStderr(), providers, names)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(infos) == 0 {
				_, err = fmt.Fprintln(out, "no sandboxes created")
				return err
			}
			if listOutputIsTTY(out) {
				res, err := runListInteractive(cmd.Context(), providers, infos)
				if err != nil {
					return err
				}
				return handleListPostAction(cmd, providers, res)
			}
			return writeListPlain(out, infos)
		},
	}
}
