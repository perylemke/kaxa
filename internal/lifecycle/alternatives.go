package lifecycle

import (
	"errors"
	"strings"

	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

// Alternatives returns the sorted names of the available providers that
// implement feature. The list comes from the registry, never a fixed set.
func Alternatives(reg *registry.Registry, feature sandbox.Feature) []string {
	var names []string
	for _, name := range reg.Names() {
		p, err := reg.Get(name)
		if err != nil || !sandbox.IsAvailable(p) {
			continue
		}
		if sandbox.CapabilitiesOf(p).Has(feature) {
			names = append(names, name)
		}
	}
	return names
}

// WithAlternatives rewrites the hint of an unsupported-feature error so it
// names the providers that do implement the feature. Other errors pass through.
func WithAlternatives(reg *registry.Registry, err error) error {
	ue, ok := errors.AsType[*sandbox.UnsupportedError](err)
	if !ok {
		return err
	}
	hint := "no other provider supports it yet"
	if names := Alternatives(reg, ue.Feature); len(names) > 0 {
		hint = "available for: " + strings.Join(names, ", ")
	}
	return usererr.Wrap(ue, ue.Error(), hint)
}
