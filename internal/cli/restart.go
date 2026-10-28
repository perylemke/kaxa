package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
)

func newRestartCmd(reg *registry.Registry) *cobra.Command {
	return newSandboxActionCmd(reg, sandboxActionSpec{
		feature: sandbox.FeatureRestart,
		verb:    "restart",
		past:    "restarted",
		gerund:  "restarting",
		title:   "Restart",
	}, func(ctx context.Context, p sandbox.Provider, id sandbox.ID) error {
		r, ok := p.(sandbox.Restarter)
		if !ok {
			return lifecycle.Unsupported(p, sandbox.FeatureRestart)
		}
		return r.Restart(ctx, id)
	})
}
