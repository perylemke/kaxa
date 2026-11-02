//go:build integration

package blaxel

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

// These tests follow the provider flows through the public Provider API, with
// in-memory fakes instead of the Blaxel API, so they cost nothing.

func infoHas(infos []sandbox.Info, id sandbox.ID) bool {
	return slices.ContainsFunc(infos, func(i sandbox.Info) bool { return i.ID == id })
}

func TestBlaxelLifecycleFlow(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	api.handler = func(spec runSpec) fakeResult {
		if len(spec.argv) > 0 && spec.argv[0] == "pwd" {
			return fakeResult{stdout: blaxelHome + "\n"}
		}
		return fakeResult{}
	}
	p := newTestProvider(t, api)
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
	if out.String() != blaxelHome+"\n" {
		t.Fatalf("exec output = %q", out.String())
	}
	if err := p.Destroy(ctx, id); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if _, err := p.Inspect(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("inspect after destroy err = %v", err)
	}
}

func TestBlaxelCreateAPIThroughLifecycle(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
	res, err := lifecycle.Create(context.Background(), p, lifecycle.Request{
		Name: "svc", API: true, APIPort: 8000, APICmd: serviceArgv,
	}, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	detail, err := p.Inspect(context.Background(), res.ID)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if detail.URL == "" || detail.Auth != sandbox.URLAuthPublic {
		t.Fatalf("detail = %+v, want a public url", detail)
	}
}

func TestBlaxelRejectsUnsupportedFeaturesThroughLifecycle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		req  lifecycle.Request
		want string
	}{
		{name: "running", req: lifecycle.Request{Name: boxName, Running: true}, want: "running is not available for blaxel"},
		{name: "public url", req: lifecycle.Request{Name: boxName, Public: true}, want: "public urls is not available for blaxel"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			api := newFakeAPI()
			p := newTestProvider(t, api)
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
