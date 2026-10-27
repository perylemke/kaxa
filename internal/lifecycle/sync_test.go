package lifecycle_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/sandbox"
)

const stepSync = "sync"

// syncProvider is a fullProvider whose sandbox carries labels and may already
// hold the repository.
type syncProvider struct {
	*fullProvider
	labels     []string
	exists     bool
	existsErr  error
	inspectErr error
}

func (s *syncProvider) Inspect(context.Context, sandbox.ID) (sandbox.Detail, error) {
	s.rec.steps = append(s.rec.steps, "inspect")
	return sandbox.Detail{ID: boxName, Labels: s.labels}, s.inspectErr
}

func (s *syncProvider) RepoExists(_ context.Context, _ sandbox.ID, _ string) (bool, error) {
	s.rec.steps = append(s.rec.steps, "exists")
	return s.exists, s.existsErr
}

func newSyncProvider() (*syncProvider, *recorder) {
	full, rec := newFull()
	return &syncProvider{fullProvider: full, labels: []string{"kaxa", sandbox.SyncLabel}}, rec
}

func TestSyncCopiesThenInstalls(t *testing.T) {
	t.Parallel()

	p, rec := newSyncProvider()
	root, name := gitRepo(t)
	writeFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.27\n")

	res, err := lifecycle.Sync(context.Background(), p, lifecycle.SyncRequest{Name: boxName, Root: root, Repo: name}, nil)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	want := []string{"inspect", "exists", stepSync, "setup"}
	if !slices.Equal(rec.steps, want) {
		t.Fatalf("steps = %v, want %v", rec.steps, want)
	}
	if res.ID != boxName || res.WorkDir != "/home/box/ws/"+name {
		t.Fatalf("result = %+v", res)
	}
	if len(rec.destroyed) != 0 {
		t.Fatalf("destroyed = %v", rec.destroyed)
	}
}

func TestSyncWithoutProjectFilesSkipsInstall(t *testing.T) {
	t.Parallel()

	p, rec := newSyncProvider()
	root, name := gitRepo(t)

	if _, err := lifecycle.Sync(context.Background(), p, lifecycle.SyncRequest{Name: boxName, Root: root, Repo: name}, nil); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if want := []string{"inspect", "exists", stepSync}; !slices.Equal(rec.steps, want) {
		t.Fatalf("steps = %v, want %v", rec.steps, want)
	}
}

func TestSyncRequiresSyncLabel(t *testing.T) {
	t.Parallel()

	p, rec := newSyncProvider()
	p.labels = []string{"kaxa"}
	root, name := gitRepo(t)

	_, err := lifecycle.Sync(context.Background(), p, lifecycle.SyncRequest{Name: boxName, Root: root, Repo: name}, nil)
	if err == nil || !strings.Contains(err.Error(), "was not created with --sync") {
		t.Fatalf("err = %v", err)
	}
	if rec.synced || len(rec.destroyed) != 0 {
		t.Fatalf("synced = %v destroyed = %v", rec.synced, rec.destroyed)
	}
}

func TestSyncRefusesExistingRepository(t *testing.T) {
	t.Parallel()

	p, rec := newSyncProvider()
	p.exists = true
	root, name := gitRepo(t)

	_, err := lifecycle.Sync(context.Background(), p, lifecycle.SyncRequest{Name: boxName, Root: root, Repo: name}, nil)
	if err == nil || !strings.Contains(err.Error(), "already exists") || !strings.Contains(err.Error(), name) {
		t.Fatalf("err = %v", err)
	}
	if rec.synced {
		t.Fatal("archive was copied over an existing directory")
	}
}

func TestSyncStopsWhenSandboxCannotBeChecked(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(*syncProvider)
		want  string
	}{
		{"inspect fails", func(p *syncProvider) { p.inspectErr = errors.New("not found") }, "not found"},
		{"exists check fails", func(p *syncProvider) { p.existsErr = errors.New("probe failed") }, "probe failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, rec := newSyncProvider()
			tt.setup(p)
			root, name := gitRepo(t)

			_, err := lifecycle.Sync(context.Background(), p, lifecycle.SyncRequest{Name: boxName, Root: root, Repo: name}, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
			if rec.synced {
				t.Fatal("archive was copied after a failed check")
			}
		})
	}
}

func TestSyncFailureKeepsSandbox(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(*syncProvider)
		want  string
	}{
		{"copy fails", func(p *syncProvider) { p.syncErr = errors.New("copy failed") }, "copy failed"},
		{"install fails", func(p *syncProvider) { p.setupErr = errors.New("install failed") }, "install failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, rec := newSyncProvider()
			tt.setup(p)
			root, name := gitRepo(t)
			writeFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.27\n")

			_, err := lifecycle.Sync(context.Background(), p, lifecycle.SyncRequest{Name: boxName, Root: root, Repo: name}, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
			if len(rec.destroyed) != 0 {
				t.Fatalf("sandbox was destroyed: %v", rec.destroyed)
			}
		})
	}
}

func TestSyncRejectsProviderWithoutSupport(t *testing.T) {
	t.Parallel()

	full, _ := newFull()
	tests := []struct {
		name string
		p    sandbox.Provider
	}{
		{"no syncer", basicProvider{rec: &recorder{}}},
		{"no repo check", full},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, name := gitRepo(t)

			_, err := lifecycle.Sync(context.Background(), tt.p, lifecycle.SyncRequest{Name: boxName, Root: root, Repo: name}, nil)
			if !errors.Is(err, sandbox.ErrUnsupported) {
				t.Fatalf("err = %v, want ErrUnsupported", err)
			}
		})
	}
}

func TestSyncReportsEachSlowStep(t *testing.T) {
	t.Parallel()

	p, _ := newSyncProvider()
	root, name := gitRepo(t)
	writeFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.27\n")

	var labels []string
	run := func(ctx context.Context, label string, task func(context.Context) error) error {
		labels = append(labels, label)
		return task(ctx)
	}
	if _, err := lifecycle.Sync(context.Background(), p, lifecycle.SyncRequest{Name: boxName, Root: root, Repo: name}, run); err != nil {
		t.Fatalf("sync: %v", err)
	}
	want := []string{"syncing " + name + "…", "installing " + name + "…"}
	if !slices.Equal(labels, want) {
		t.Fatalf("labels = %v, want %v", labels, want)
	}
}
