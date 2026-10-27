package sandbox

// WorkspaceLayout reports where a provider keeps files inside a sandbox.
type WorkspaceLayout interface {
	// HomeDir is the default working directory of a sandbox.
	HomeDir() string
	// RepoDir is the directory that SyncWorkspace fills for repo.
	// It rejects a repo name that is not a single path segment.
	RepoDir(repo string) (string, error)
}

// Availability reports whether a provider can create sandboxes today.
// A provider that does not implement it is treated as available.
type Availability interface {
	Available() bool
}

// IsAvailable reports whether p can serve sandbox requests.
func IsAvailable(p Provider) bool {
	a, ok := p.(Availability)
	return !ok || a.Available()
}

// Capabilities lists the optional features a provider implements.
type Capabilities struct {
	Sync      bool `json:"sync"`
	Setup     bool `json:"setup"`
	API       bool `json:"api"`
	PublicURL bool `json:"public_url"`
	Layout    bool `json:"workspace_layout"`
}

// CapabilitiesOf inspects which optional interfaces p implements.
func CapabilitiesOf(p Provider) Capabilities {
	_, sync := p.(WorkspaceSyncer)
	_, setup := p.(ProjectSetup)
	_, api := p.(APIPublisher)
	_, public := p.(PublicURL)
	_, layout := p.(WorkspaceLayout)
	return Capabilities{Sync: sync, Setup: setup, API: api, PublicURL: public, Layout: layout}
}
