package docker

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

const (
	restoredVersion  = 1
	restoredFile     = "docker-restored.json"
	restoredDirMode  = 0o700
	restoredFileMode = 0o600
)

// restoredEntry is a sandbox that Docker forked from a snapshot. Docker gives
// it no labels, and labels cannot be added later, so kaxa keeps this record to
// know the sandbox is its own and which labels the source had.
type restoredEntry struct {
	// Resource is the API resource name, such as sandboxes/abc. It never
	// repeats, so an entry outlives a rename and never matches another sandbox.
	Resource string   `json:"resource"`
	Name     string   `json:"name"`
	Labels   []string `json:"labels,omitempty"`
}

type restoredDocument struct {
	Version int             `json:"version"`
	Sandbox []restoredEntry `json:"sandboxes"`
}

// restoredIndex is a JSON file of restored sandboxes, private to the current
// user. It is per machine and is not locked across processes.
type restoredIndex struct {
	mu   sync.Mutex
	path string
}

func newRestoredIndex(path string) *restoredIndex {
	return &restoredIndex{path: path}
}

// defaultRestoredPath is <user config dir>/kaxa/docker-restored.json.
func defaultRestoredPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find config directory: %w", err)
	}
	return filepath.Join(dir, "kaxa", restoredFile), nil
}

// load reads the entries. A missing file is an empty index.
func (x *restoredIndex) load() ([]restoredEntry, error) {
	//gosec:disable G304 -- path is the fixed index file under the user config directory
	data, err := os.ReadFile(x.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read restored sandboxes index: %w", err)
	}
	var doc restoredDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse restored sandboxes index %s: %w", x.path, err)
	}
	if doc.Version != restoredVersion {
		return nil, fmt.Errorf("restored sandboxes index %s has unsupported version %d", x.path, doc.Version)
	}
	return doc.Sandbox, nil
}

// save writes the entries atomically: a temporary file, then a rename.
func (x *restoredIndex) save(entries []restoredEntry) error {
	if entries == nil {
		entries = []restoredEntry{}
	}
	data, err := json.MarshalIndent(restoredDocument{Version: restoredVersion, Sandbox: entries}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode restored sandboxes index: %w", err)
	}
	dir := filepath.Dir(x.path)
	if err := os.MkdirAll(dir, restoredDirMode); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".docker-restored-*")
	if err != nil {
		return fmt.Errorf("write restored sandboxes index: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(restoredFileMode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write restored sandboxes index: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write restored sandboxes index: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write restored sandboxes index: %w", err)
	}
	if err := os.Rename(tmp.Name(), x.path); err != nil {
		return fmt.Errorf("write restored sandboxes index: %w", err)
	}
	return nil
}

// labels returns the labels of every restored sandbox, by resource name.
func (x *restoredIndex) labels() (map[string][]string, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	entries, err := x.load()
	if err != nil {
		return nil, err
	}
	out := make(map[string][]string, len(entries))
	for _, e := range entries {
		out[e.Resource] = e.Labels
	}
	return out, nil
}

// add records a restored sandbox, replacing an entry of the same resource.
func (x *restoredIndex) add(e restoredEntry) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	entries, err := x.load()
	if err != nil {
		return err
	}
	entries = slices.DeleteFunc(entries, func(o restoredEntry) bool { return o.Resource == e.Resource })
	return x.save(append(entries, e))
}

// remove forgets a sandbox. One that is not recorded is not an error.
func (x *restoredIndex) remove(resource string) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	entries, err := x.load()
	if err != nil {
		return err
	}
	kept := slices.DeleteFunc(slices.Clone(entries), func(o restoredEntry) bool { return o.Resource == resource })
	if len(kept) == len(entries) {
		return nil
	}
	return x.save(kept)
}
