package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/perylemke/sbox/internal/registry"
	"github.com/perylemke/sbox/internal/sandbox"
)

var sboxCreateLabels = []string{"sbox"}

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
			name := args[0]
			spec := sandbox.Spec{Name: name, Labels: sboxCreateLabels}
			id, err := runCreateWithSpinner(cmd.Context(), cmd.ErrOrStderr(), name, func(ctx context.Context) (sandbox.ID, error) {
				return p.Create(ctx, spec)
			})
			if err != nil {
				return err
			}
			return WriteSuccess(cmd.OutOrStdout(), fmt.Sprintf("sandbox %s is ready", id))
		},
	}
}
