package cli

import (
	"github.com/spf13/cobra"

	"github.com/perylemke/sbox/internal/usererr"
)

func atMostOneName(command string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		return rejectExtraName(command, args)
	}
}

func rejectExtraName(command string, args []string) error {
	if len(args) <= 1 {
		return nil
	}
	return usererr.New(
		command+" accepts one sandbox name",
		"sbox "+command+" [name]",
	)
}

func noArgs(command string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			return nil
		}
		return usererr.New(
			command+" does not take arguments",
			"sbox "+command,
		)
	}
}
