package cli

import "github.com/spf13/cobra"

func skipConfigLoad(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "help", "completion", "version":
		return true
	}
	if help, err := cmd.Flags().GetBool("help"); err == nil && help {
		return true
	}
	return false
}
