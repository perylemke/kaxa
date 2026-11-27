package vercel

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
	indexFile     = "vercel-snapshots.json"
	indexDirMode  = 0o700
	indexFileMode = 0o600
)

// snapshotEntry is the comment Vercel does not store on a snapshot.
type snapshotEntry struct {
	ID      string `json:"id"`
	Sandbox string `json:"sandbox"`
	Comment string `json:"comment,omitempty"`
}

type indexDocument struct {
	Version   int             `json:"version"`
	Snapshots []snapshotEntry `json:"snapshots"`
}

// snapshotIndex is a JSON file of snapshot comments, private to this machine.
type snapshotIndex struct {
	mu   sync.Mutex
	path string
}

func newSnapshotIndex(path string) *snapshotIndex {
	return &snapshotIndex{path: path}
}

func defaultIndexPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find config directory: %w", err)
	}
	return filepath.Join(dir, "kaxa", indexFile), nil
}

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
	tmp, err := os.CreateTemp(dir, ".vercel-snapshots-*")
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

func (x *snapshotIndex) all() (map[string]snapshotEntry, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	entries, err := x.load()
	if err != nil {
		return nil, err
	}
	out := make(map[string]snapshotEntry, len(entries))
	for _, e := range entries {
		out[e.ID] = e
	}
	return out, nil
}

func (x *snapshotIndex) add(e snapshotEntry) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	entries, err := x.load()
	if err != nil {
		return err
	}
	entries = slices.DeleteFunc(entries, func(o snapshotEntry) bool { return o.ID == e.ID })
	return x.save(append(entries, e))
}

// removeIf drops every entry for which drop is true and saves only on change.
func (x *snapshotIndex) removeIf(drop func(snapshotEntry) bool) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	entries, err := x.load()
	if err != nil {
		return err
	}
	kept := slices.DeleteFunc(slices.Clone(entries), drop)
	if len(kept) == len(entries) {
		return nil
	}
	return x.save(kept)
}
