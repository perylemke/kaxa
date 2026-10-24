package cli

import (
	"github.com/spf13/cobra"

	"github.com/perylemke/sbox/internal/registry"
)

func newListCmd(reg *registry.Registry) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List sandboxes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := providerFromCmd(cmd, reg)
			if err != nil {
				return err
			}
			infos, err := p.List(cmd.Context())
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if listOutputIsTTY(out) {
				return writeListTTY(out, infos)
			}
			return writeListPlain(out, infos)
		},
	}
}
