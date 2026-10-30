//go:build integration

package e2b_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/provider/e2b"
	"github.com/perylemke/kaxa/internal/sandbox"
)

// These tests follow the flows of e2b_live_test.go through the public Provider
// API, with in-memory fakes instead of the E2B API, so they cost nothing. Run
// the live tests to check against the real service.

func infoHas(infos []sandbox.Info, id sandbox.ID) bool {
	return slices.ContainsFunc(infos, func(i sandbox.Info) bool { return i.ID == id })
}

func freshProvider() (*e2b.Provider, *fakeAPI, *fakeEnvd) {
	api, envd := newFakeAPI(), &fakeEnvd{}
	p := e2b.NewWithClient(api, envd, testKey)
	p.SetReadiness(time.Second, time.Millisecond)
	return p, api, envd
}

func TestE2BLifecycleFlow(t *testing.T) {
	t.Parallel()

	p, api, envd := freshProvider()
	ctx := context.Background()

	id, err := p.Create(ctx, sandbox.Spec{Name: testBox, Labels: []string{"kaxa", sandbox.SyncLabel}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := p.Create(ctx, sandbox.Spec{Name: testBox}); err == nil || !strings.Contains(err.Error(), "already exists") {
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
	envd.script = func(e2b.StartRequest) *fakeProc { return output("/home/user/workspace\n", "", 0) }
	if err := p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{"pwd"}, Stdout: &out}); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if out.String() != "/home/user/workspace\n" {
		t.Fatalf("exec output = %q", out.String())
	}

	if err := p.Restart(ctx, id); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if err := p.Destroy(ctx, id); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if _, err := p.Inspect(ctx, id); !errors.Is(err, e2b.ErrNotFound) {
		t.Fatalf("inspect after destroy err = %v", err)
	}
	if infos, err := p.List(ctx); err != nil || infoHas(infos, id) {
		t.Fatalf("list after destroy = %+v err = %v", infos, err)
	}
	if len(api.killed) != 1 {
		t.Fatalf("killed = %v", api.killed)
	}
}

func TestE2BCreateAPIThroughLifecycle(t *testing.T) {
	t.Parallel()

	p, api, envd := freshProvider()
	var probes int
	envd.script = serviceScript(&probes, 1, &fakeProc{block: make(chan struct{})})

	res, err := lifecycle.Create(context.Background(), p, lifecycle.Request{
		Name: "svc", API: true, APIPort: 8000, APICmd: []string{"python", "-m", "http.server", "8000"},
	}, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(api.creates) != 1 || !api.creates[0].PublicTraffic || api.creates[0].APIPort != 8000 {
		t.Fatalf("create params = %+v, want a public sandbox for port 8000", api.creates)
	}
	detail, err := p.Inspect(context.Background(), res.ID)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if want := "https://8000-" + api.boxes["svc"].ID + ".e2b.app"; detail.URL != want {
		t.Fatalf("url = %q, want %q", detail.URL, want)
	}
}

func TestE2BRejectsUnsupportedFeaturesThroughLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  lifecycle.Request
		want string
	}{
		{name: "running", req: lifecycle.Request{Name: "box", Running: true}, want: "running is not available for e2b"},
		{name: "public url", req: lifecycle.Request{Name: "box", Public: true}, want: "public urls is not available for e2b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, api, _ := freshProvider()
			_, err := lifecycle.Create(context.Background(), p, tt.req, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
			if len(api.creates) != 0 {
				t.Fatalf("created %v before rejecting", api.creates)
			}
		})
	}
}

func TestE2BTemplateThroughLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		defaultName string
		req         string
		want        string
	}{
		{name: "from the request", req: testBig, want: testBig},
		{name: "from the default", defaultName: testTeam, want: testTeam},
		{name: "request over default", defaultName: testTeam, req: testBig, want: testBig},
		{name: "base", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, api, _ := freshProvider()
			if err := p.SetDefaultTemplate(tt.defaultName); err != nil {
				t.Fatalf("set default: %v", err)
			}
			_, err := lifecycle.Create(context.Background(), p, lifecycle.Request{Name: "tpl", Template: tt.req}, nil)
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			if got := api.creates[0].Template; got != tt.want {
				t.Fatalf("template = %q, want %q", got, tt.want)
			}
		})
	}
}
