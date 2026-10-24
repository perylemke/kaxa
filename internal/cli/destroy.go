package cli

import (
	"github.com/spf13/cobra"

	"github.com/perylemke/sbox/internal/registry"
	"github.com/perylemke/sbox/internal/sandbox"
)

func newDestroyCmd(reg *registry.Registry) *cobra.Command {
	return &cobra.Command{
		Use:     "destroy <name>",
		Aliases: []string{"rm"},
		Short:   "Destroy a sandbox",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := providerFromCmd(cmd, reg)
			if err != nil {
				return err
			}
			return p.Destroy(cmd.Context(), sandbox.ID(args[0]))
		},
	}
}
