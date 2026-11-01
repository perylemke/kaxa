package daytona

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
	indexVersion  = 1
	indexFile     = "daytona-snapshots.json"
	indexDirMode  = 0o700
	indexFileMode = 0o600
)

// snapshotEntry is what Daytona cannot store on an image: the comment of the
// snapshot and the labels of the sandbox it came from. A restored sandbox gets
// the labels back, so it stays a kaxa sandbox.
type snapshotEntry struct {
	// Image is the image name, which is also the snapshot ID.
	Image   string   `json:"image"`
	Comment string   `json:"comment,omitempty"`
	Labels  []string `json:"labels,omitempty"`
}

type indexDocument struct {
	Version   int             `json:"version"`
	Snapshots []snapshotEntry `json:"snapshots"`
}

// snapshotIndex is a JSON file of snapshot entries, private to the current
// user. It is per machine and is not locked across processes.
type snapshotIndex struct {
	mu   sync.Mutex
	path string
}

func newSnapshotIndex(path string) *snapshotIndex {
	return &snapshotIndex{path: path}
}

// defaultIndexPath is <user config dir>/kaxa/daytona-snapshots.json.
func defaultIndexPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find config directory: %w", err)
	}
	return filepath.Join(dir, "kaxa", indexFile), nil
}

// load reads the entries. A missing file is an empty index.
func (x *snapshotIndex) load() ([]snapshotEntry, error) {
	//gosec:disable G304 -- path is the fixed index file under the user config directory
	data, err := os.ReadFile(x.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read snapshot index: %w", err)
	}
	var doc indexDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse snapshot index %s: %w", x.path, err)
	}
	if doc.Version != indexVersion {
		return nil, fmt.Errorf("snapshot index %s has unsupported version %d", x.path, doc.Version)
	}
	return doc.Snapshots, nil
}

// save writes the entries atomically: a temporary file, then a rename.
func (x *snapshotIndex) save(entries []snapshotEntry) error {
	if entries == nil {
		entries = []snapshotEntry{}
	}
	data, err := json.MarshalIndent(indexDocument{Version: indexVersion, Snapshots: entries}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode snapshot index: %w", err)
	}
	dir := filepath.Dir(x.path)
	if err := os.MkdirAll(dir, indexDirMode); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".daytona-snapshots-*")
	if err != nil {
		return fmt.Errorf("write snapshot index: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Chmod(indexFileMode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write snapshot index: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write snapshot index: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write snapshot index: %w", err)
	}
	if err := os.Rename(tmp.Name(), x.path); err != nil {
		return fmt.Errorf("write snapshot index: %w", err)
	}
	return nil
}

// all returns the entries by image name.
func (x *snapshotIndex) all() (map[string]snapshotEntry, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	entries, err := x.load()
	if err != nil {
		return nil, err
	}
	out := make(map[string]snapshotEntry, len(entries))
	for _, e := range entries {
		out[e.Image] = e
	}
	return out, nil
}

// add records a snapshot, replacing an entry of the same image.
func (x *snapshotIndex) add(e snapshotEntry) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	entries, err := x.load()
	if err != nil {
		return err
	}
	entries = slices.DeleteFunc(entries, func(o snapshotEntry) bool { return o.Image == e.Image })
	return x.save(append(entries, e))
}

// remove forgets a snapshot. One that is not recorded is not an error.
func (x *snapshotIndex) remove(image string) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	entries, err := x.load()
	if err != nil {
		return err
	}
	kept := slices.DeleteFunc(slices.Clone(entries), func(o snapshotEntry) bool { return o.Image == image })
	if len(kept) == len(entries) {
		return nil
	}
	return x.save(kept)
}
