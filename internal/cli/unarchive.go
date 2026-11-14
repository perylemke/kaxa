package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
)

func newUnarchiveCmd(reg *registry.Registry) *cobra.Command {
	return newSandboxActionCmd(reg, sandboxActionSpec{
		feature: sandbox.FeatureUnarchive,
		verb:    "unarchive",
		past:    "unarchived",
		gerund:  "unarchiving",
		title:   "Unarchive",
	}, func(ctx context.Context, p sandbox.Provider, id sandbox.ID) error {
		u, ok := p.(sandbox.Unarchiver)
		if !ok {
			return lifecycle.Unsupported(p, sandbox.FeatureUnarchive)
		}
		return u.Unarchive(ctx, id)
	})
}
