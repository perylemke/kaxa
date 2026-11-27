package vercel

import (
	"context"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestLayout(t *testing.T) {
	t.Parallel()
	p := New()
	if p.HomeDir() != vercelHome {
		t.Fatalf("HomeDir() = %q", p.HomeDir())
	}
	got, err := p.RepoDir("app")
	if err != nil || got != workspaceDir+"/app" {
		t.Fatalf("RepoDir() = %q, %v", got, err)
	}
	for _, bad := range []string{"", ".", "..", "a/b", `a\b`, "a\x00b"} {
		if _, err := p.RepoDir(bad); err == nil {
			t.Errorf("RepoDir(%q) accepted", bad)
		}
	}
}

func TestPrepareWorkspace(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	if err := p.PrepareWorkspace(context.Background(), boxName); err != nil {
		t.Fatal(err)
	}
	if len(f.mkdirs) != 1 || f.mkdirs[0] != workspaceDir {
		t.Fatalf("mkdirs = %v", f.mkdirs)
	}
	if f.body("POST /v2/sandboxes/sessions/{sid}/fs/mkdir", 0)["recursive"] != true {
		t.Fatal("mkdir is not recursive")
	}
	f.fail2("POST /v2/sandboxes/sessions/{sid}/fs/mkdir", 500)
	if err := p.PrepareWorkspace(context.Background(), boxName); err == nil {
		t.Fatal("mkdir failure ignored")
	}
	if err := p.PrepareWorkspace(context.Background(), "missing"); err == nil {
		t.Fatal("missing sandbox accepted")
	}
}

func TestSyncWorkspace(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	ctx := context.Background()
	if err := p.SyncWorkspace(ctx, boxName, strings.NewReader("tar-bytes")); err != nil {
		t.Fatal(err)
	}
	if string(f.tar) != "tar-bytes" || f.tarCwd != workspaceDir {
		t.Fatalf("tar = %q in %q", f.tar, f.tarCwd)
	}
	if err := p.SyncWorkspace(ctx, boxName, nil); err == nil {
		t.Fatal("nil archive accepted")
	}
	if err := p.SyncWorkspace(ctx, "missing", strings.NewReader("x")); err == nil {
		t.Fatal("missing sandbox accepted")
	}
	f.fail2("POST /v2/sandboxes/sessions/{sid}/fs/write", 413)
	if err := p.SyncWorkspace(ctx, boxName, strings.NewReader("x")); err == nil || !strings.Contains(err.Error(), "upload workspace") {
		t.Fatalf("write failure = %v", err)
	}
	f.fail2("POST /v2/sandboxes/sessions/{sid}/fs/mkdir", 500)
	if err := p.SyncWorkspace(ctx, boxName, strings.NewReader("x")); err == nil || !strings.Contains(err.Error(), "create workspace") {
		t.Fatalf("mkdir failure = %v", err)
	}
}

func TestRepoExists(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		code    int
		want    bool
		wantErr bool
	}{
		{"present", 0, true, false},
		{"absent", 1, false, false},
		{"test failed", 2, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, f := newTestProvider(t)
			createBox(t, p)
			f.exec = func(*fakeCmd) fakeResult { return fakeResult{code: tt.code} }
			got, err := p.RepoExists(context.Background(), boxName, "app")
			if got != tt.want || (err != nil) != tt.wantErr {
				t.Fatalf("RepoExists() = %v, %v", got, err)
			}
			args := asList(cmdOf(f, 0)["args"])
			if args[len(args)-1] != workspaceDir+"/app" || args[len(args)-3] != "test" {
				t.Fatalf("args = %v", args)
			}
		})
	}
	t.Run("rejects", func(t *testing.T) {
		t.Parallel()
		p, _ := newTestProvider(t)
		createBox(t, p)
		if _, err := p.RepoExists(context.Background(), boxName, "a/b"); err == nil {
			t.Fatal("bad repo accepted")
		}
		if _, err := p.RepoExists(context.Background(), "missing", "app"); err == nil {
			t.Fatal("missing sandbox accepted")
		}
	})
	t.Run("api failure", func(t *testing.T) {
		t.Parallel()
		p, f := newTestProvider(t)
		createBox(t, p)
		f.fail2("POST /v2/sandboxes/sessions/{sid}/cmd", 500)
		if _, err := p.RepoExists(context.Background(), boxName, "app"); err == nil {
			t.Fatal("API failure read as an answer")
		}
	})
}

func TestSetupProjects(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	ctx := context.Background()
	if err := p.SetupProjects(ctx, boxName, "a/b", nil); err == nil {
		t.Fatal("bad repo accepted")
	}
	if err := p.SetupProjects(ctx, boxName, "app", nil); err != nil {
		t.Fatalf("no steps: %v", err)
	}
	if f.calls("POST /v2/sandboxes/sessions/{sid}/cmd") != 0 {
		t.Fatal("no steps still ran commands")
	}
	if err := p.SetupProjects(ctx, "missing", "app", []projectStep{{}}); err == nil {
		t.Fatal("missing sandbox accepted")
	}
}

func TestSessionRunner(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	s, err := p.open(context.Background(), boxName)
	if err != nil {
		t.Fatal(err)
	}
	f.exec = func(*fakeCmd) fakeResult { return fakeResult{stdout: "v1"} }
	out, err := sessionRunner{s}.Run(context.Background(), "/w", []string{"PATH=/x"}, []string{"go", "version"}, nil)
	if err != nil || out != "v1" {
		t.Fatalf("Run() = %q, %v", out, err)
	}
	body := cmdOf(f, 0)
	if body["command"] != "go" || body["cwd"] != "/w" || asMap(body["env"])["PATH"] != "/x" {
		t.Fatalf("body = %v", body)
	}
}

var _ sandbox.WorkspaceSyncer = (*Provider)(nil)
