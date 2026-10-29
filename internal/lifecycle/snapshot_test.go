package lifecycle_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

type noSnapProvider struct{}

func (noSnapProvider) Name() string { return testPlain }
func (noSnapProvider) Create(context.Context, sandbox.Spec) (sandbox.ID, error) {
	return "", nil
}
func (noSnapProvider) List(context.Context) ([]sandbox.Info, error) { return nil, nil }
func (noSnapProvider) Inspect(context.Context, sandbox.ID) (sandbox.Detail, error) {
	return sandbox.Detail{}, nil
}

func (noSnapProvider) Exec(context.Context, sandbox.ID, sandbox.ExecOptions) error {
	return nil
}
func (noSnapProvider) Destroy(context.Context, sandbox.ID) error { return nil }

func TestSnapshotterOfUnsupported(t *testing.T) {
	t.Parallel()

	_, err := lifecycle.SnapshotterOf(noSnapProvider{})
	if !errors.Is(err, sandbox.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
	ue, ok := errors.AsType[*sandbox.UnsupportedError](err)
	if !ok || ue.Feature != sandbox.FeatureSnapshot {
		t.Fatalf("unsupported error = %+v", ue)
	}
}

func TestValidateSnapshotRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mode    sandbox.RestoreMode
		req     sandbox.RestoreRequest
		wantErr bool
	}{
		{"in place ok", sandbox.RestoreInPlace, sandbox.RestoreRequest{Sandbox: "a", Snapshot: "v1"}, false},
		{"in place rejects target", sandbox.RestoreInPlace, sandbox.RestoreRequest{Sandbox: "a", Snapshot: "v1", Target: "b"}, true},
		{"new sandbox ok", sandbox.RestoreNewSandbox, sandbox.RestoreRequest{Sandbox: "a", Snapshot: testImage, Target: "b"}, false},
		{"new sandbox needs target", sandbox.RestoreNewSandbox, sandbox.RestoreRequest{Sandbox: "a", Snapshot: testImage}, true},
		{"new sandbox same target", sandbox.RestoreNewSandbox, sandbox.RestoreRequest{Sandbox: "a", Snapshot: testImage, Target: "a"}, true},
		{"bad snapshot id", sandbox.RestoreInPlace, sandbox.RestoreRequest{Sandbox: "a", Snapshot: "../x"}, true},
		{"unknown mode", sandbox.RestoreMode("x"), sandbox.RestoreRequest{Sandbox: "a", Snapshot: "v1"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := lifecycle.ValidateSnapshotRequest(tt.mode, tt.req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// snapProv is a provider with sandboxes and snapshots in memory.
type snapProv struct {
	noSnapProvider
	name      string
	boxes     []sandbox.ID
	snaps     map[sandbox.ID][]sandbox.Snapshot
	listErr   error
	snapErr   error
	snapCalls []sandbox.ID
}

func (p *snapProv) Name() string { return p.name }
func (p *snapProv) List(context.Context) ([]sandbox.Info, error) {
	if p.listErr != nil {
		return nil, p.listErr
	}
	out := make([]sandbox.Info, 0, len(p.boxes))
	for _, id := range p.boxes {
		out = append(out, sandbox.Info{ID: id, Provider: p.name})
	}
	return out, nil
}
func (p *snapProv) RestoreMode() sandbox.RestoreMode { return sandbox.RestoreInPlace }
func (p *snapProv) Snapshot(context.Context, sandbox.ID, string) (sandbox.Snapshot, error) {
	return sandbox.Snapshot{}, nil
}

func (p *snapProv) ListSnapshots(_ context.Context, id sandbox.ID) ([]sandbox.Snapshot, error) {
	p.snapCalls = append(p.snapCalls, id)
	if p.snapErr != nil {
		return nil, p.snapErr
	}
	return p.snaps[id], nil
}

func (p *snapProv) Restore(context.Context, sandbox.RestoreRequest) (sandbox.ID, error) {
	return "", nil
}

func (p *snapProv) DeleteSnapshot(context.Context, sandbox.ID, sandbox.SnapshotID) error {
	return nil
}

// catalogProv also lists every snapshot at once, even of sandboxes that are gone.
type catalogProv struct {
	*snapProv
	all []sandbox.Snapshot
}

func (p catalogProv) ListAllSnapshots(context.Context) ([]sandbox.Snapshot, error) {
	return p.all, nil
}

func at(minute int) time.Time { return time.Date(2026, 1, 1, 0, minute, 0, 0, time.UTC) }

func snapIDs(snaps []sandbox.Snapshot) []string {
	out := make([]string, 0, len(snaps))
	for _, s := range snaps {
		out = append(out, s.Provider+"/"+string(s.Sandbox)+"/"+string(s.ID))
	}
	return out
}

func TestCollectSnapshots(t *testing.T) {
	t.Parallel()

	newSprites := func() *snapProv {
		return &snapProv{
			name:  listSprites,
			boxes: []sandbox.ID{testWeb, testAPI},
			snaps: map[sandbox.ID][]sandbox.Snapshot{
				testWeb: {{ID: "v1", CreatedAt: at(1)}, {ID: "v2", CreatedAt: at(2)}},
				testAPI: {{ID: "v1", CreatedAt: at(3)}},
			},
		}
	}
	newModal := func() catalogProv {
		return catalogProv{
			snapProv: &snapProv{name: listModal, boxes: []sandbox.ID{testWeb}},
			all: []sandbox.Snapshot{
				{ID: testImage, Sandbox: testWeb, CreatedAt: at(4)},
				{ID: "im-2", Sandbox: "gone", CreatedAt: at(5)},
			},
		}
	}

	tests := []struct {
		name    string
		sandbox sandbox.ID
		want    []string
	}{
		{"all sandboxes, provider order, then sandbox, newest first", "", []string{
			"sprites/api/v1", "sprites/web/v2", "sprites/web/v1", "modal/gone/im-2", "modal/web/im-1",
		}},
		{"one sandbox is found in every provider", testWeb, []string{
			"sprites/web/v2", "sprites/web/v1", "modal/web/im-1",
		}},
		{"a sandbox that only a catalog still knows", "gone", []string{"modal/gone/im-2"}},
		{"unknown sandbox", "nope", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			providers := map[string]sandbox.Provider{listSprites: newSprites(), listModal: newModal()}
			got, failures, err := lifecycle.CollectSnapshots(context.Background(), providers, []string{listSprites, listModal}, tt.sandbox)
			if err != nil || len(failures) != 0 {
				t.Fatalf("err = %v, failures = %v", err, failures)
			}
			if !slices.Equal(snapIDs(got), tt.want) {
				t.Fatalf("snapshots = %v, want %v", snapIDs(got), tt.want)
			}
		})
	}
}

func TestCollectSnapshotsAsksOnlyTheNamedSandbox(t *testing.T) {
	t.Parallel()

	p := &snapProv{name: listSprites, boxes: []sandbox.ID{testWeb, testAPI}, snaps: map[sandbox.ID][]sandbox.Snapshot{}}
	_, _, err := lifecycle.CollectSnapshots(context.Background(), map[string]sandbox.Provider{listSprites: p}, []string{listSprites}, testWeb)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !slices.Equal(p.snapCalls, []sandbox.ID{testWeb}) {
		t.Fatalf("listed snapshots of %v, want only web", p.snapCalls)
	}
	p.snapCalls = nil
	_, _, _ = lifecycle.CollectSnapshots(context.Background(), map[string]sandbox.Provider{listSprites: p}, []string{listSprites}, "missing")
	if len(p.snapCalls) != 0 {
		t.Fatalf("listed snapshots of %v for a sandbox the provider does not have", p.snapCalls)
	}
}

func TestCollectSnapshotsFailures(t *testing.T) {
	t.Parallel()

	good := func() *snapProv {
		return &snapProv{name: listModal, boxes: []sandbox.ID{"a"}, snaps: map[sandbox.ID][]sandbox.Snapshot{"a": {{ID: testImage, CreatedAt: at(1)}}}}
	}
	boom := errors.New("boom")

	t.Run("one provider failing is a warning", func(t *testing.T) {
		t.Parallel()
		bad := &snapProv{name: listSprites, listErr: boom}
		got, failures, err := lifecycle.CollectSnapshots(context.Background(),
			map[string]sandbox.Provider{listSprites: bad, listModal: good()}, []string{listSprites, listModal}, "")
		if err != nil || len(got) != 1 || len(failures) != 1 || !errors.Is(failures[0], boom) {
			t.Fatalf("got = %v, failures = %v, err = %v", got, failures, err)
		}
	})
	t.Run("a failing snapshot list is the failure of its provider", func(t *testing.T) {
		t.Parallel()
		bad := &snapProv{name: listSprites, boxes: []sandbox.ID{"a"}, snapErr: boom}
		_, failures, err := lifecycle.CollectSnapshots(context.Background(),
			map[string]sandbox.Provider{listSprites: bad, listModal: good()}, []string{listSprites, listModal}, "")
		if err != nil || len(failures) != 1 {
			t.Fatalf("failures = %v, err = %v", failures, err)
		}
	})
	t.Run("every provider failing is an error", func(t *testing.T) {
		t.Parallel()
		bad := &snapProv{name: listSprites, listErr: boom}
		_, _, err := lifecycle.CollectSnapshots(context.Background(), map[string]sandbox.Provider{listSprites: bad}, []string{listSprites}, "")
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want boom", err)
		}
	})
	t.Run("no provider with snapshots is a user error", func(t *testing.T) {
		t.Parallel()
		_, _, err := lifecycle.CollectSnapshots(context.Background(), map[string]sandbox.Provider{testPlain: noSnapProvider{}}, []string{testPlain}, "")
		if _, ok := usererr.As(err); !ok {
			t.Fatalf("err = %v, want a user error", err)
		}
	})
	t.Run("a provider without snapshots is skipped", func(t *testing.T) {
		t.Parallel()
		got, failures, err := lifecycle.CollectSnapshots(context.Background(),
			map[string]sandbox.Provider{testPlain: noSnapProvider{}, listModal: good()}, []string{testPlain, listModal}, "")
		if err != nil || len(failures) != 0 || len(got) != 1 {
			t.Fatalf("got = %v, failures = %v, err = %v", got, failures, err)
		}
	})
}

const (
	testPlain = "plain"
	testImage = "im-1"
	testAPI   = "api"
	testWeb   = "web"
)
