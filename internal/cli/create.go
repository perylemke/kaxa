package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/perylemke/sbox/internal/registry"
	"github.com/perylemke/sbox/internal/sandbox"
)

func newCreateCmd(reg *registry.Registry) *cobra.Command {
	return &cobra.Command{
		Use:   "create <name>",
		Short: "Create a sandbox",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := providerFromCmd(cmd, reg)
			if err != nil {
				return err
			}
			id, err := p.Create(cmd.Context(), sandbox.Spec{Name: args[0]})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), id)
			return err
		},
	}
}
