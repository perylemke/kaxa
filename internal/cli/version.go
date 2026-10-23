package cli

import (
	"github.com/spf13/cobra"

	"github.com/perylemke/sbx/internal/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the sbx version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return version.Write(cmd.OutOrStdout())
		},
	}
}
