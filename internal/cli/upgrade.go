package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
)

func newUpgradeCmd(reg *registry.Registry) *cobra.Command {
	return newSandboxActionCmd(reg, sandboxActionSpec{
		verb:   "upgrade",
		past:   "upgraded",
		gerund: "upgrading",
		title:  "Upgrade",
	}, func(ctx context.Context, p sandbox.Provider, id sandbox.ID) error {
		return p.Upgrade(ctx, id)
	})
}
