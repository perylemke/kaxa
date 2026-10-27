package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
)

// listTargets returns the providers that list queries, keyed by name, and their
// names in registry order. With --provider it is that one provider. Without it,
// every available provider that has credentials; the default provider setting
// does not narrow it.
func listTargets(cmd *cobra.Command, reg *registry.Registry) (map[string]sandbox.Provider, []string, error) {
	if cmd.Flags().Changed("provider") {
		p, err := providerFromCmd(cmd, reg)
		if err != nil {
			return nil, nil, err
		}
		return map[string]sandbox.Provider{p.Name(): p}, []string{p.Name()}, nil
	}
	cfg, err := configFromContext(cmd.Context())
	if err != nil {
		return nil, nil, err
	}
	return lifecycle.ListTargets(reg, cfg)
}

// collectSandboxes lists every named provider, in order. A provider that fails
// is reported on errOut and skipped while another succeeds; if all fail, the
// errors are returned.
func collectSandboxes(ctx context.Context, errOut io.Writer, providers map[string]sandbox.Provider, names []string) ([]sandbox.Info, error) {
	infos, failures, err := lifecycle.CollectSandboxes(ctx, providers, names)
	if err != nil {
		return nil, err
	}
	for _, failure := range failures {
		if _, werr := fmt.Fprintf(errOut, "warning: could not list %v\n", failure); werr != nil {
			return nil, werr
		}
	}
	return infos, nil
}
