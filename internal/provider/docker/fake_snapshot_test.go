package docker

import (
	"context"
	"fmt"
	"slices"
	"time"
)

const (
	testSnapshotAt = "20260102030405"
	snapEtag       = `"s1"`
)

// fakeSnapshots is the in-memory snapshot part of fakeAPI. A new snapshot is
// creating until it is read once, which makes the provider poll.
type fakeSnapshots struct {
	snaps map[string]*SnapshotRecord
	n     int
	// stuck keeps a new snapshot in creating; broken ends it in failed.
	stuck, broken bool
	// unlabeled makes a restore create a sandbox without kaxa's labels.
	unlabeled bool
	// partial makes the answer to a restore leave the labels out, while the
	// sandbox itself has them.
	partial  bool
	created  []string
	restored []string
}

func (f *fakeAPI) snapState() *fakeSnapshots {
	if f.snapshots == nil {
		f.snapshots = &fakeSnapshots{snaps: map[string]*SnapshotRecord{}}
	}
	return f.snapshots
}

// addSnapshot puts a ready snapshot in the fake without going through the API.
func (f *fakeAPI) addSnapshot(id, displayName, description string, at time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.snapState()
	res := "snapshots/" + id
	s.snaps[res] = &SnapshotRecord{
		ID: id, Resource: res, DisplayName: displayName, Description: description,
		Status: snapshotReady, Etag: snapEtag, CreatedAt: at,
	}
}

func (f *fakeAPI) CreateSnapshot(_ context.Context, r Record, displayName, description string) (SnapshotRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("create-snapshot"); err != nil {
		return SnapshotRecord{}, err
	}
	if rec, ok := f.recs[r.Resource]; !ok || rec.Status != statusRunning {
		return SnapshotRecord{}, &APIError{Status: 409, Code: "failedPrecondition", Message: "sandbox is not running"}
	}
	s := f.snapState()
	s.n++
	id := fmt.Sprintf("snap%d", s.n)
	res := "snapshots/" + id
	snap := &SnapshotRecord{
		ID: id, Resource: res, DisplayName: displayName, Description: description,
		Status: snapshotCreating, Etag: snapEtag, CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	s.snaps[res] = snap
	s.created = append(s.created, displayName)
	return *snap, nil
}

func (f *fakeAPI) ListSnapshots(context.Context) ([]SnapshotRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("list-snapshots"); err != nil {
		return nil, err
	}
	var out []SnapshotRecord
	for _, snap := range f.snapState().snaps {
		listed := *snap
		// The list view carries no description.
		listed.Description = ""
		out = append(out, listed)
	}
	slices.SortFunc(out, func(a, b SnapshotRecord) int { return a.CreatedAt.Compare(b.CreatedAt) })
	return out, nil
}

func (f *fakeAPI) GetSnapshot(_ context.Context, resource string) (SnapshotRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("get-snapshot"); err != nil {
		return SnapshotRecord{}, err
	}
	snap, ok := f.snapState().snaps[resource]
	if !ok {
		return SnapshotRecord{}, ErrSnapshotNotFound
	}
	out := *snap
	if snap.Status == snapshotCreating && !f.snapState().stuck {
		if f.snapState().broken {
			snap.Status, snap.Failure = snapshotFailed, "disk full"
		} else {
			snap.Status = snapshotReady
		}
	}
	return out, nil
}

func (f *fakeAPI) DeleteSnapshot(_ context.Context, s SnapshotRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("delete-snapshot"); err != nil {
		return err
	}
	if _, ok := f.snapState().snaps[s.Resource]; !ok {
		return ErrSnapshotNotFound
	}
	delete(f.snapState().snaps, s.Resource)
	return nil
}

func (f *fakeAPI) RestoreSnapshot(_ context.Context, s SnapshotRecord, name string) (Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.record("restore-snapshot"); err != nil {
		return Record{}, err
	}
	if _, ok := f.snapState().snaps[s.Resource]; !ok {
		return Record{}, ErrSnapshotNotFound
	}
	f.n++
	res := fmt.Sprintf("sandboxes/id%d", f.n)
	r := &Record{
		Name: name, Resource: res, Status: statusNew, Labels: []string{labelKaxa},
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Etag: `"e1"`,
		Endpoint: Endpoint{URI: "https://sbx.example.test/" + res}, managed: !f.snapState().unlabeled,
	}
	f.recs[res] = r
	f.next[res] = statusRunning
	f.snapState().restored = append(f.snapState().restored, name)
	out := *r
	if f.snapState().partial {
		out.managed = false
	}
	return out, nil
}
