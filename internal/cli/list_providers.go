package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
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
	providers := map[string]sandbox.Provider{}
	var names []string
	for _, name := range reg.Names() {
		p, err := lifecycle.ResolveProvider(reg, cfg, name)
		if errors.Is(err, sandbox.ErrNotConfigured) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		if !sandbox.IsAvailable(p) {
			continue
		}
		providers[name] = p
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil, nil, usererr.New(
			"no provider is configured",
			"set KAXA_SPRITES_TOKEN, or KAXA_MODAL_TOKEN_ID and KAXA_MODAL_TOKEN_SECRET",
		)
	}
	return providers, names, nil
}

// collectSandboxes lists every named provider, in order. A provider that fails
// is reported on errOut and skipped while another succeeds; if all fail, the
// errors are returned.
func collectSandboxes(ctx context.Context, errOut io.Writer, providers map[string]sandbox.Provider, names []string) ([]sandbox.Info, error) {
	var (
		all  []sandbox.Info
		errs []error
		ok   int
	)
	for _, name := range names {
		infos, err := providers[name].List(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		ok++
		for _, info := range infos {
			if info.Provider == "" {
				info.Provider = name
			}
			all = append(all, info)
		}
	}
	if ok == 0 {
		return nil, errors.Join(errs...)
	}
	for _, err := range errs {
		if _, werr := fmt.Fprintf(errOut, "warning: could not list %v\n", err); werr != nil {
			return nil, werr
		}
	}
	return all, nil
}
