package registry

import (
	"fmt"
	"sort"
	"sync"

	"github.com/perylemke/sbx/internal/sandbox"
)

// Registry maps provider names to implementations.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]sandbox.Provider
}

// New returns an empty registry.
func New() *Registry {
	return &Registry{
		providers: make(map[string]sandbox.Provider),
	}
}

// Register adds a provider. Duplicate names return an error.
func (r *Registry) Register(p sandbox.Provider) error {
	if p == nil {
		return fmt.Errorf("register provider: nil")
	}
	name := p.Name()
	if name == "" {
		return fmt.Errorf("register provider: empty name")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[name]; exists {
		return fmt.Errorf("register provider: %q already registered", name)
	}
	r.providers[name] = p
	return nil
}

// Get returns a provider by name.
func (r *Registry) Get(name string) (sandbox.Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider %q", name)
	}
	return p, nil
}

// Names returns registered provider names in sorted order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Has reports whether name is registered.
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.providers[name]
	return ok
}
