//go:build integration

package vercel

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// TestIntegrationLifecycle runs the whole lifecycle against the in-memory fake:
// create, sync, exec, snapshot, restore, and destroy. It needs no credentials.
func TestIntegrationLifecycle(t *testing.T) {
	p, f := newTestProvider(t)
	ctx := context.Background()
	f.exec = func(c *fakeCmd) fakeResult { return fakeResult{stdout: "ok\n"} }

	id, err := p.Create(ctx, sandbox.Spec{Name: boxName, Labels: []string{"kaxa", sandbox.SyncLabel}})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.PrepareWorkspace(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := p.SyncWorkspace(ctx, id, strings.NewReader("archive")); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{"echo", "ok"}, Stdout: &out}); err != nil || out.String() != "ok\n" {
		t.Fatalf("Exec() = %q, %v", out.String(), err)
	}
	snap, err := p.Snapshot(ctx, id, "checkpoint")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := p.Restore(ctx, sandbox.RestoreRequest{Sandbox: id, Snapshot: snap.ID}); err != nil || got != id {
		t.Fatalf("Restore() = %q, %v", got, err)
	}
	infos, err := p.List(ctx)
	if err != nil || len(infos) != 1 || infos[0].Status != "running" {
		t.Fatalf("List() = %+v, %v", infos, err)
	}
	if err := p.Destroy(ctx, id); err != nil {
		t.Fatal(err)
	}
	if infos, _ := p.List(ctx); len(infos) != 0 {
		t.Fatalf("List() after destroy = %+v", infos)
	}
}
