package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/perylemke/kaxa/internal/config"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

// ListTargets returns every available provider that has credentials, keyed by
// name, and their names in registry order. The default provider setting does
// not narrow it. It is a user error when no provider is usable.
func ListTargets(reg *registry.Registry, cfg config.Config) (map[string]sandbox.Provider, []string, error) {
	providers := map[string]sandbox.Provider{}
	var names []string
	for _, name := range reg.Names() {
		p, err := ResolveProvider(reg, cfg, name)
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
			"set KAXA_SPRITES_TOKEN, KAXA_E2B_API_KEY, or KAXA_MODAL_TOKEN_ID and KAXA_MODAL_TOKEN_SECRET, or KAXA_DOCKER_ID and KAXA_DOCKER_PAT for docker",
		)
	}
	return providers, names, nil
}

// CollectSandboxes lists every named provider, in order. A provider that fails
// is returned in failures and skipped while another succeeds; if all fail, err
// joins their errors.
func CollectSandboxes(ctx context.Context, providers map[string]sandbox.Provider, names []string) (infos []sandbox.Info, failures []error, err error) {
	ok := 0
	for _, name := range names {
		listed, listErr := providers[name].List(ctx)
		if listErr != nil {
			failures = append(failures, fmt.Errorf("%s: %w", name, listErr))
			continue
		}
		ok++
		for _, info := range listed {
			if info.Provider == "" {
				info.Provider = name
			}
			infos = append(infos, info)
		}
	}
	if ok == 0 {
		return nil, nil, errors.Join(failures...)
	}
	return infos, failures, nil
}
