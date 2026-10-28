package sandbox

import (
	"errors"
	"fmt"
)

// ErrUnsupported indicates a provider has no equivalent of a requested feature.
var ErrUnsupported = errors.New("unsupported")

// Feature names an optional capability that a provider may lack.
type Feature string

// Optional features, worded for messages: "<feature> is not available for <provider>".
const (
	FeatureRestart           Feature = "restart"
	FeatureUpgrade           Feature = "upgrade"
	FeatureSync              Feature = "sync"
	FeatureDependencyInstall Feature = "dependency install"
	FeatureAPIPublish        Feature = "api publish"
	FeaturePublicURLs        Feature = "public urls"
	FeatureURLAuth           Feature = "url auth"
	FeatureRunning           Feature = "running"
)

// UnsupportedError reports that a provider does not offer a feature.
type UnsupportedError struct {
	Provider string
	Feature  Feature
}

func (e *UnsupportedError) Error() string {
	return fmt.Sprintf("%s is not available for %s", e.Feature, e.Provider)
}

// Unwrap lets errors.Is match ErrUnsupported.
func (e *UnsupportedError) Unwrap() error {
	return ErrUnsupported
}

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
	Restart   bool `json:"restart"`
	Upgrade   bool `json:"upgrade"`
	Running   bool `json:"running"`
}

// Has reports whether the capabilities cover feature.
func (c Capabilities) Has(feature Feature) bool {
	switch feature {
	case FeatureRestart:
		return c.Restart
	case FeatureUpgrade:
		return c.Upgrade
	case FeatureRunning:
		return c.Running
	case FeatureSync:
		return c.Sync
	case FeatureDependencyInstall:
		return c.Setup
	case FeatureAPIPublish:
		return c.API
	case FeaturePublicURLs, FeatureURLAuth:
		return c.PublicURL
	default:
		return false
	}
}

// CapabilitiesOf inspects which optional interfaces p implements.
func CapabilitiesOf(p Provider) Capabilities {
	_, sync := p.(WorkspaceSyncer)
	_, setup := p.(ProjectSetup)
	_, api := p.(APIPublisher)
	_, public := p.(PublicURL)
	_, layout := p.(WorkspaceLayout)
	_, restart := p.(Restarter)
	_, upgrade := p.(Upgrader)
	_, running := p.(RunningSupport)
	return Capabilities{
		Sync: sync, Setup: setup, API: api, PublicURL: public, Layout: layout,
		Restart: restart, Upgrade: upgrade, Running: running,
	}
}
