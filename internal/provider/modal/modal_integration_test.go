//go:build integration

package modal_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/provider/modal"
	"github.com/perylemke/kaxa/internal/sandbox"
)

// These tests follow the flows of modal_live_test.go through the public
// Provider API, with the in-memory fakeClient instead of the Modal API, so
// they cost nothing. Run the live tests to check against the real service.

func listHas(infos []sandbox.Info, id sandbox.ID) bool {
	return slices.ContainsFunc(infos, func(i sandbox.Info) bool { return i.ID == id })
}

func TestModalCRUD(t *testing.T) {
	t.Parallel()

	fake := newFakeClient()
	p := newProvider(t, fake)
	ctx := context.Background()

	sid, err := p.Create(ctx, sandbox.Spec{Name: testName, Labels: []string{testKaxa, "integration"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := p.Create(ctx, sandbox.Spec{Name: testName}); !errors.Is(err, modal.ErrAlreadyExists) {
		t.Fatalf("duplicate create err = %v, want ErrAlreadyExists", err)
	}

	infos, err := p.List(ctx)
	if err != nil || !listHas(infos, sid) {
		t.Fatalf("list = %+v, err = %v", infos, err)
	}
	detail, err := p.Inspect(ctx, sid)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if detail.ID != sid || detail.Status != "running" || len(detail.Labels) != 2 {
		t.Fatalf("detail = %+v", detail)
	}

	assertModalExecFlows(t, fake, p, sid)

	if err := p.Destroy(ctx, sid); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if _, err := p.Inspect(ctx, sid); !errors.Is(err, modal.ErrNotFound) {
		t.Fatalf("inspect after destroy err = %v, want ErrNotFound", err)
	}
	if infos, err = p.List(ctx); err != nil || listHas(infos, sid) {
		t.Fatalf("list after destroy = %+v, err = %v", infos, err)
	}
}

func assertModalExecFlows(t *testing.T, fake *fakeClient, p *modal.Provider, sid sandbox.ID) {
	t.Helper()
	ctx := context.Background()

	fake.process = newFakeProcess("hi\n", "oops\n", 0)
	var stdout, stderr lockedBuffer
	script := []string{"sh", "-c", "echo hi; echo oops >&2"}
	if err := p.Exec(ctx, sid, sandbox.ExecOptions{Command: script, Stdout: &stdout, Stderr: &stderr}); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if stdout.String() != "hi\n" || stderr.String() != "oops\n" || !slices.Equal(fake.execArgv, script) {
		t.Fatalf("stdout = %q, stderr = %q, argv = %q", stdout.String(), stderr.String(), fake.execArgv)
	}

	fake.process = newFakeProcess("", "", 3)
	err := p.Exec(ctx, sid, sandbox.ExecOptions{Command: []string{"sh", "-c", "exit 3"}})
	if exitErr, ok := errors.AsType[*sandbox.ExitError](err); !ok || exitErr.Code != 3 {
		t.Fatalf("exec exit 3 err = %v, want exit code 3", err)
	}

	fake.process = newFakeProcess("30 100\n", "", 0)
	var tty lockedBuffer
	err = p.Exec(ctx, sid, sandbox.ExecOptions{
		Command: []string{"sh", "-c", "stty size"}, Stdout: &tty, TTY: true, Size: sandbox.TermSize{Rows: 30, Cols: 100},
	})
	if err != nil {
		t.Fatalf("exec tty: %v", err)
	}
	if !fake.execPTY || !slices.Contains(fake.execArgv, "30") || !slices.Contains(fake.execArgv, "100") {
		t.Fatalf("tty = %v, argv = %q, want the size applied", fake.execPTY, fake.execArgv)
	}
}

func TestModalSync(t *testing.T) {
	t.Parallel()

	fake := newFakeClient()
	p := newProvider(t, fake)
	ctx := context.Background()

	sid, err := p.Create(ctx, sandbox.Spec{Name: testName, Labels: []string{testKaxa, sandbox.SyncLabel}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !fake.withTools {
		t.Fatal("a sandbox with the sync label was not built from the sync image")
	}

	if err := p.SyncWorkspace(ctx, sid, strings.NewReader("TARBALL")); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := fake.process.stdin.String(); got != "TARBALL" {
		t.Fatalf("archive sent = %q", got)
	}

	if err := p.Exec(ctx, sid, sandbox.ExecOptions{Command: []string{"pwd"}}); err != nil {
		t.Fatalf("exec: %v", err)
	}
	assertRunsAsModalIn(t, fake, testWorkspace)

	steps := []project.Step{{Kind: project.KindGo, Commands: [][]string{{"go", "version"}}}}
	if err := p.SetupProjects(ctx, sid, "demo", steps); err != nil {
		t.Fatalf("setup: %v", err)
	}
	assertRunsAsModalIn(t, fake, testWorkspace+"/demo")
}

// assertRunsAsModalIn checks the last command ran as the modal user in dir.
func assertRunsAsModalIn(t *testing.T, fake *fakeClient, dir string) {
	t.Helper()
	last := fake.execs[len(fake.execs)-1]
	if len(last.argv) < 3 || last.argv[0] != testRunuser || last.argv[2] != testModalUser || last.dir != dir {
		t.Fatalf("last command = %q in %q, want it as modal in %q", last.argv, last.dir, dir)
	}
}

func TestModalSnapshotRoundTrip(t *testing.T) {
	t.Parallel()

	fake := newFakeClient()
	p := newSnapshotProvider(t, fake)
	ctx := context.Background()

	src := sandbox.ID(testName)
	addBox(t, p, testName, testKaxa, "integration")
	snap, err := p.Snapshot(ctx, src, "integration")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	list, err := p.ListSnapshots(ctx, src)
	if err != nil || len(list) != 1 || list[0].ID != snap.ID || list[0].Comment != "integration" {
		t.Fatalf("list = %+v, err = %v", list, err)
	}

	dst := sandbox.ID(testName + "-copy")
	restored, err := p.Restore(ctx, sandbox.RestoreRequest{Sandbox: src, Snapshot: snap.ID, Target: dst})
	if err != nil || restored != dst {
		t.Fatalf("restore = %q, %v", restored, err)
	}
	if len(fake.fromImage) != 1 || fake.fromImage[0].name != string(dst) || fake.fromImage[0].image != string(snap.ID) {
		t.Fatalf("restore created %+v, want %q from image %q", fake.fromImage, dst, snap.ID)
	}
	if _, ok := fake.boxes[testName]; !ok {
		t.Fatal("the source sandbox was not kept")
	}

	if err := p.DeleteSnapshot(ctx, src, snap.ID); err != nil {
		t.Fatalf("delete snapshot: %v", err)
	}
	if !slices.Equal(fake.deletedImages, []string{string(snap.ID)}) {
		t.Fatalf("deleted images = %v", fake.deletedImages)
	}
	if list, _ := p.ListSnapshots(ctx, src); len(list) != 0 {
		t.Fatalf("list after delete = %+v", list)
	}
}

func TestModalSnapshotKeepsSyncUser(t *testing.T) {
	t.Parallel()

	fake := newFakeClient()
	p := newSnapshotProvider(t, fake)
	ctx := context.Background()

	src := sandbox.ID(testName)
	addBox(t, p, testName, testKaxa, sandbox.SyncLabel)
	snap, err := p.Snapshot(ctx, src, "")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	dst := sandbox.ID(testName + "-copy")
	if _, err := p.Restore(ctx, sandbox.RestoreRequest{Sandbox: src, Snapshot: snap.ID, Target: dst}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if len(fake.fromImage) != 1 || !slices.Contains(fake.fromImage[0].labels, sandbox.SyncLabel) {
		t.Fatalf("restore labels = %+v, want the sync label kept", fake.fromImage)
	}

	if err := p.Exec(ctx, dst, sandbox.ExecOptions{Command: []string{"pwd"}}); err != nil {
		t.Fatalf("exec: %v", err)
	}
	assertRunsAsModalIn(t, fake, testWorkspace)
}

func TestModalRejectsBadToken(t *testing.T) {
	t.Parallel()

	fake := newFakeClient()
	fake.failWith = errors.New("unauthorized for " + testTokenSecret)
	_, err := newProvider(t, fake).List(context.Background())
	if err == nil || errors.Is(err, modal.ErrMissingCredentials) {
		t.Fatalf("list err = %v, want an API error", err)
	}
	if strings.Contains(err.Error(), testTokenSecret) {
		t.Fatalf("err = %q leaks the token secret", err)
	}
}
