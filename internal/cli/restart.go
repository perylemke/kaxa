package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/perylemke/sbox/internal/registry"
	"github.com/perylemke/sbox/internal/sandbox"
)

func newRestartCmd(reg *registry.Registry) *cobra.Command {
	return newSandboxActionCmd(reg, sandboxActionSpec{
		verb:   "restart",
		past:   "restarted",
		gerund: "restarting",
		title:  "Restart",
	}, func(ctx context.Context, p sandbox.Provider, id sandbox.ID) error {
		return p.Restart(ctx, id)
	})
}
