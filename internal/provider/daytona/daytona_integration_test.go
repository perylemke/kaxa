//go:build integration

package daytona

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/sandbox"
)

// These tests follow the flows of daytona_live_test.go through the public
// Provider API, with in-memory fakes instead of the Daytona API, so they cost
// nothing. Run the live tests to check against the real service.

func infoHas(infos []sandbox.Info, id sandbox.ID) bool {
	return slices.ContainsFunc(infos, func(i sandbox.Info) bool { return i.ID == id })
}

func TestDaytonaLifecycleFlow(t *testing.T) {
	t.Parallel()

	tb := newFakeToolbox(func(req runSpec, _ string) fakeResult {
		if req.argv[0] == "pwd" {
			return fakeResult{stdout: daytonaHome + "\n"}
		}
		return fakeResult{}
	})
	p, api := newTestProvider(t, tb)
	ctx := context.Background()

	id, err := p.Create(ctx, sandbox.Spec{Name: boxName, Labels: []string{"kaxa", sandbox.SyncLabel}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := p.Create(ctx, sandbox.Spec{Name: boxName}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate create err = %v", err)
	}
	if infos, err := p.List(ctx); err != nil || !infoHas(infos, id) {
		t.Fatalf("list = %+v err = %v", infos, err)
	}

	if err := p.SyncWorkspace(ctx, id, strings.NewReader("TARBALL")); err != nil {
		t.Fatalf("sync: %v", err)
	}
	steps := []project.Step{{Kind: project.KindGo, Commands: [][]string{{"go", "build", "./..."}}}}
	if err := p.SetupProjects(ctx, id, "demo", steps); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if ok, err := p.RepoExists(ctx, id, "demo"); err != nil || !ok {
		t.Fatalf("repo exists = %v err = %v", ok, err)
	}

	var out strings.Builder
	if err := p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{"pwd"}, Stdout: &out}); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if out.String() != daytonaHome+"\n" {
		t.Fatalf("exec output = %q", out.String())
	}

	if err := p.Restart(ctx, id); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if err := p.Destroy(ctx, id); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if _, err := p.Inspect(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("inspect after destroy err = %v", err)
	}
	if infos, err := p.List(ctx); err != nil || infoHas(infos, id) {
		t.Fatalf("list after destroy = %+v err = %v", infos, err)
	}
	if !api.has("delete sb-1") {
		t.Fatalf("sandbox not deleted: %v", api.calls)
	}
}

func TestDaytonaCreateAPIThroughLifecycle(t *testing.T) {
	t.Parallel()

	tb := newFakeToolbox(func(req runSpec, _ string) fakeResult {
		return fakeResult{running: isService(req)}
	})
	p, api := newTestProvider(t, tb)

	res, err := lifecycle.Create(context.Background(), p, lifecycle.Request{
		Name: "svc", API: true, APIPort: 8000, APICmd: serviceArgv,
	}, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !api.has("setport sb-1 8000") {
		t.Fatalf("port not published: %v", api.calls)
	}
	detail, err := p.Inspect(context.Background(), res.ID)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if detail.URL == "" || detail.Auth != sandbox.URLAuthPublic {
		t.Fatalf("detail = %+v, want a public url", detail)
	}
}

func TestDaytonaRejectsUnsupportedFeaturesThroughLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  lifecycle.Request
		want string
	}{
		{name: "running", req: lifecycle.Request{Name: boxName, Running: true}, want: "running is not available for daytona"},
		{name: "public url", req: lifecycle.Request{Name: boxName, Public: true}, want: "public urls is not available for daytona"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, api := newTestProvider(t, newFakeToolbox(okHandler))
			_, err := lifecycle.Create(context.Background(), p, tt.req, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
			if len(api.sandboxes) != 0 {
				t.Fatalf("created %d sandboxes before rejecting", len(api.sandboxes))
			}
		})
	}
}

func TestDaytonaSnapshotFlow(t *testing.T) {
	t.Parallel()

	p, api := newTestProvider(t, newFakeToolbox(okHandler))
	ctx := context.Background()
	if _, err := p.Create(ctx, sandbox.Spec{Name: boxName, Labels: []string{sandbox.SyncLabel}}); err != nil {
		t.Fatalf("create: %v", err)
	}

	snap, err := p.Snapshot(ctx, boxName, "baseline")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snaps, err := p.ListSnapshots(ctx, boxName); err != nil || len(snaps) != 1 || snaps[0].Comment != "baseline" {
		t.Fatalf("snapshots = %+v err = %v", snaps, err)
	}
	if all, err := p.ListAllSnapshots(ctx); err != nil || len(all) != 1 {
		t.Fatalf("all snapshots = %+v err = %v", all, err)
	}

	if _, err := p.Restore(ctx, sandbox.RestoreRequest{Sandbox: boxName, Snapshot: snap.ID, Target: "copy"}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if infos, err := p.List(ctx); err != nil || len(infos) != 2 {
		t.Fatalf("list after restore = %+v err = %v", infos, err)
	}

	if err := p.DeleteSnapshot(ctx, boxName, snap.ID); err != nil {
		t.Fatalf("delete snapshot: %v", err)
	}
	if snaps, err := p.ListSnapshots(ctx, boxName); err != nil || len(snaps) != 0 {
		t.Fatalf("snapshots after delete = %+v err = %v", snaps, err)
	}
	if len(api.images) != 0 {
		t.Fatalf("images left: %+v", api.images)
	}
}
