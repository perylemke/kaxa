package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
)

func newUpgradeCmd(reg *registry.Registry) *cobra.Command {
	return newSandboxActionCmd(reg, sandboxActionSpec{
		feature: sandbox.FeatureUpgrade,
		verb:    "upgrade",
		past:    "upgraded",
		gerund:  "upgrading",
		title:   "Upgrade",
	}, func(ctx context.Context, p sandbox.Provider, id sandbox.ID) error {
		u, ok := p.(sandbox.Upgrader)
		if !ok {
			return lifecycle.Unsupported(p, sandbox.FeatureUpgrade)
		}
		return u.Upgrade(ctx, id)
	})
}
