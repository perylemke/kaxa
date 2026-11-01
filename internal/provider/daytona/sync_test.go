package daytona

import (
	"slices"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestLayout(t *testing.T) {
	t.Parallel()
	p := New()
	if p.HomeDir() != "/home/daytona" {
		t.Fatalf("HomeDir = %q", p.HomeDir())
	}
	dir, err := p.RepoDir("app")
	if err != nil || dir != "/home/daytona/workspace/app" {
		t.Fatalf("RepoDir = %q, %v", dir, err)
	}
	for _, bad := range []string{"", ".", "..", "a/b", `a\b`, "a\x00b"} {
		if _, err := p.RepoDir(bad); err == nil {
			t.Errorf("RepoDir(%q) accepted", bad)
		}
	}
}

func TestSyncWorkspaceUploadsExtractsAndCleansUp(t *testing.T) {
	t.Parallel()
	tb := newFakeToolbox(okHandler)
	p, api := newTestProvider(t, tb)
	api.add(boxName, statusRunning)
	if err := p.SyncWorkspace(t.Context(), boxName, strings.NewReader("tarball")); err != nil {
		t.Fatalf("SyncWorkspace: %v", err)
	}
	if len(tb.uploads) != 1 {
		t.Fatalf("uploads = %v", tb.uploads)
	}
	var tmp string
	for path, data := range tb.uploads {
		tmp = path
		if data != "tarball" || !strings.HasPrefix(path, "/tmp/kaxa-sync-") {
			t.Fatalf("upload %q = %q", path, data)
		}
	}
	want := [][]string{
		{"mkdir", "-p", workspaceDir},
		{"tar", "-xzf", tmp, "-C", workspaceDir},
		{"rm", "-f", tmp},
	}
	got := tb.argvs()
	if len(got) != len(want) {
		t.Fatalf("commands = %v", got)
	}
	for i := range want {
		if !slices.Equal(got[i], want[i]) {
			t.Errorf("command %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestSyncWorkspaceFailsCleanly(t *testing.T) {
	t.Parallel()
	tb := newFakeToolbox(func(req ProcessRequest, _ string) fakeResult {
		if plain(req.Argv)[0] == "tar" {
			return fakeResult{code: 2, stderr: "not a tar"}
		}
		return fakeResult{}
	})
	p, api := newTestProvider(t, tb)
	api.add(boxName, statusRunning)
	err := p.SyncWorkspace(t.Context(), boxName, strings.NewReader("x"))
	if err == nil || !strings.Contains(err.Error(), "not a tar") {
		t.Fatalf("err = %v", err)
	}
	if last := tb.argvs()[len(tb.argvs())-1]; last[0] != "rm" {
		t.Fatalf("temporary archive was not removed: %v", last)
	}
	if err := p.SyncWorkspace(t.Context(), boxName, nil); err == nil {
		t.Fatal("nil archive accepted")
	}
}

func TestRepoExists(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		code    int
		want    bool
		wantErr bool
	}{
		{"present", 0, true, false},
		{"absent", 1, false, false},
		{"failure", 2, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tb := newFakeToolbox(func(ProcessRequest, string) fakeResult { return fakeResult{code: tt.code} })
			p, api := newTestProvider(t, tb)
			api.add(boxName, statusRunning)
			got, err := p.RepoExists(t.Context(), boxName, "app")
			if got != tt.want || (err != nil) != tt.wantErr {
				t.Fatalf("RepoExists = %v, %v", got, err)
			}
			if cmd := tb.argvs()[0]; !slices.Equal(cmd, []string{"test", "-d", workspaceDir + "/app"}) {
				t.Fatalf("command = %v", cmd)
			}
		})
	}
	p, _ := newTestProvider(t, newFakeToolbox(okHandler))
	if _, err := p.RepoExists(t.Context(), boxName, "../x"); err == nil {
		t.Fatal("traversal accepted")
	}
}

func TestSetupProjects(t *testing.T) {
	t.Parallel()
	tb := newFakeToolbox(okHandler)
	p, api := newTestProvider(t, tb)
	api.add(boxName, statusRunning, sandbox.SyncLabel)
	if err := p.SetupProjects(t.Context(), boxName, "app", nil); err != nil {
		t.Fatalf("no steps: %v", err)
	}
	if len(tb.reqs) != 0 {
		t.Fatal("empty setup ran commands")
	}
	steps := []project.Step{{Kind: project.KindGo, Commands: [][]string{{"go", "mod", "download"}}}}
	if err := p.SetupProjects(t.Context(), boxName, "app", steps); err != nil {
		t.Fatalf("SetupProjects: %v", err)
	}
	found := false
	for _, r := range tb.reqs {
		if slices.Equal(plain(r.Argv), []string{"go", "mod", "download"}) && r.Dir == workspaceDir+"/app" {
			found = true
		}
	}
	if !found {
		t.Fatalf("project command not run in the repository: %v", tb.argvs())
	}
	if err := p.SetupProjects(t.Context(), boxName, "a/b", steps); err == nil {
		t.Fatal("bad repo accepted")
	}
}
