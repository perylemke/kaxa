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
	FeatureTemplate          Feature = "template"
	FeatureSnapshot          Feature = "snapshot"
	FeatureCPU               Feature = "cpu"
	FeatureMemory            Feature = "memory"
	FeatureNetworkBlock      Feature = "network block"
	FeatureDiskPercent       Feature = "disk percent"
	FeatureUnarchive         Feature = "unarchive"
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

// SyncRequiresCreate marks a provider whose sandbox only has a workspace when it
// was created with sync. Modal is the one: its tools image, which holds the
// workspace, is chosen at create time. Other providers prepare the workspace
// for every sandbox, so sync works on them either way.
type SyncRequiresCreate interface {
	SyncRequiresCreate() bool
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
	Sync         bool `json:"sync"`
	Setup        bool `json:"setup"`
	API          bool `json:"api"`
	PublicURL    bool `json:"public_url"`
	Layout       bool `json:"workspace_layout"`
	Restart      bool `json:"restart"`
	Upgrade      bool `json:"upgrade"`
	Running      bool `json:"running"`
	Template     bool `json:"template"`
	CPU          bool `json:"cpu"`
	Memory       bool `json:"memory"`
	NetworkBlock bool `json:"network_block"`
	Snapshot     bool `json:"snapshot"`
	// SnapshotRestore is the RestoreMode of a provider that has Snapshot.
	SnapshotRestore string `json:"snapshot_restore,omitempty"`
	DiskPercent     bool   `json:"disk_percent"`
	Unarchive       bool   `json:"unarchive"`
	PublicPreview   bool   `json:"public_preview"`
}

// Has reports whether the capabilities cover feature.
func (c Capabilities) Has(feature Feature) bool {
	known := map[Feature]bool{
		FeatureRestart:           c.Restart,
		FeatureUpgrade:           c.Upgrade,
		FeatureRunning:           c.Running,
		FeatureTemplate:          c.Template,
		FeatureSnapshot:          c.Snapshot,
		FeatureCPU:               c.CPU,
		FeatureMemory:            c.Memory,
		FeatureNetworkBlock:      c.NetworkBlock,
		FeatureSync:              c.Sync,
		FeatureDependencyInstall: c.Setup,
		FeatureAPIPublish:        c.API,
		FeaturePublicURLs:        c.PublicURL,
		FeatureURLAuth:           c.PublicURL,
		FeatureDiskPercent:       c.DiskPercent,
		FeatureUnarchive:         c.Unarchive,
	}
	return known[feature]
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
	_, template := p.(TemplateSupport)
	_, cpu := p.(CPUSupport)
	_, memory := p.(MemorySupport)
	_, networkBlock := p.(NetworkBlockSupport)
	_, disk := p.(DiskPercentSupport)
	_, unarchive := p.(Unarchiver)
	_, preview := p.(PublicPreview)
	caps := Capabilities{
		Sync: sync, Setup: setup, API: api, PublicURL: public, Layout: layout,
		Restart: restart, Upgrade: upgrade, Running: running, Template: template,
		CPU: cpu, Memory: memory, NetworkBlock: networkBlock,
		DiskPercent: disk, Unarchive: unarchive, PublicPreview: preview,
	}
	if s, ok := p.(Snapshotter); ok {
		caps.Snapshot = true
		caps.SnapshotRestore = string(s.RestoreMode())
	}
	return caps
}
