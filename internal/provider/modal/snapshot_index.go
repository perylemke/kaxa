package modal

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	// snapshotRetention is how long Modal keeps a snapshot image by default.
	snapshotRetention = 30 * 24 * time.Hour
	indexVersion      = 1
	indexFile         = "modal-snapshots.json"
	indexDirMode      = 0o700
	indexFileMode     = 0o600
)

// snapshotEntry is one snapshot image kaxa took. Modal cannot list images, so
// kaxa keeps this record itself.
type snapshotEntry struct {
	ID        string    `json:"id"`
	Sandbox   string    `json:"sandbox"`
	Comment   string    `json:"comment,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	// Labels are those of the source sandbox, restored onto the new one so a
	// synced sandbox keeps its workspace user.
	Labels []string `json:"labels,omitempty"`
}

type indexDocument struct {
	Version   int             `json:"version"`
	Snapshots []snapshotEntry `json:"snapshots"`
}

// snapshotIndex is a JSON file of snapshot entries, private to the current user.
// It is per machine and is not locked across processes.
type snapshotIndex struct {
	mu   sync.Mutex
	path string
	now  func() time.Time
}

func newSnapshotIndex(path string) *snapshotIndex {
	return &snapshotIndex{path: path, now: time.Now}
}

// defaultIndexPath is <user config dir>/kaxa/modal-snapshots.json.
func defaultIndexPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find config directory: %w", err)
	}
	return filepath.Join(dir, "kaxa", indexFile), nil
}

// load reads the entries and drops those past the retention period.
// A missing file is an empty index.
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
	cutoff := x.now().Add(-snapshotRetention)
	live := doc.Snapshots[:0]
	for _, e := range doc.Snapshots {
		if e.CreatedAt.After(cutoff) {
			live = append(live, e)
		}
	}
	return live, nil
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
	tmp, err := os.CreateTemp(dir, ".modal-snapshots-*")
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

func (x *snapshotIndex) add(e snapshotEntry) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	entries, err := x.load()
	if err != nil {
		return err
	}
	return x.save(append(entries, e))
}

// all returns every entry, newest first.
func (x *snapshotIndex) all() ([]snapshotEntry, error) {
	return x.matching(func(snapshotEntry) bool { return true })
}

// forSandbox returns the entries of one sandbox, newest first.
func (x *snapshotIndex) forSandbox(sandbox string) ([]snapshotEntry, error) {
	return x.matching(func(e snapshotEntry) bool { return e.Sandbox == sandbox })
}

func (x *snapshotIndex) matching(keep func(snapshotEntry) bool) ([]snapshotEntry, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	entries, err := x.load()
	if err != nil {
		return nil, err
	}
	var out []snapshotEntry
	for _, e := range entries {
		if keep(e) {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// find returns the entry id when it belongs to sandbox.
func (x *snapshotIndex) find(sandbox, id string) (snapshotEntry, bool, error) {
	entries, err := x.forSandbox(sandbox)
	if err != nil {
		return snapshotEntry{}, false, err
	}
	for _, e := range entries {
		if e.ID == id {
			return e, true, nil
		}
	}
	return snapshotEntry{}, false, nil
}

func (x *snapshotIndex) remove(id string) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	entries, err := x.load()
	if err != nil {
		return err
	}
	kept := entries[:0]
	for _, e := range entries {
		if e.ID != id {
			kept = append(kept, e)
		}
	}
	return x.save(kept)
}
