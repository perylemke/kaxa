package cli

import (
	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the kaxa version",
		Args:  noArgs("version"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return version.Write(cmd.OutOrStdout())
		},
	}
}
