package e2b_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/provider/e2b"
	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestLayout(t *testing.T) {
	t.Parallel()

	p := e2b.New()
	var layout sandbox.WorkspaceLayout = p
	if layout.HomeDir() != "/home/user" {
		t.Fatalf("home = %q", layout.HomeDir())
	}
	dir, err := layout.RepoDir("api")
	if err != nil || dir != "/home/user/workspace/api" {
		t.Fatalf("dir = %q err = %v", dir, err)
	}
	for _, bad := range []string{"", ".", "..", "a/b", `a\b`, "a\x00b"} {
		if _, err := layout.RepoDir(bad); err == nil {
			t.Fatalf("RepoDir(%q) accepted", bad)
		}
	}
}

func TestCapabilities(t *testing.T) {
	t.Parallel()

	caps := sandbox.CapabilitiesOf(e2b.New())
	want := sandbox.Capabilities{Sync: true, Setup: true, API: true, Layout: true, Restart: true, Template: true}
	if caps != want {
		t.Fatalf("capabilities = %+v, want %+v", caps, want)
	}
	for _, feature := range []sandbox.Feature{sandbox.FeatureUpgrade, sandbox.FeatureRunning, sandbox.FeaturePublicURLs, sandbox.FeatureSnapshot} {
		if caps.Has(feature) {
			t.Fatalf("e2b claims %q", feature)
		}
	}
}

func TestSyncWorkspaceUploadsThenExtracts(t *testing.T) {
	t.Parallel()

	p, _, envd := newProvider([]string{testKaxa, sandbox.SyncLabel}, 0)

	if err := p.SyncWorkspace(context.Background(), testBox, strings.NewReader("TARBALL")); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(envd.uploads) != 1 {
		t.Fatalf("uploads = %+v", envd.uploads)
	}
	up := envd.uploads[0]
	if up.body != "TARBALL" || up.user != testUser || !strings.HasPrefix(up.path, "/tmp/kaxa-sync-") || !strings.HasSuffix(up.path, ".tgz") {
		t.Fatalf("upload = %+v", up)
	}
	cmds := envd.argvs()
	if len(cmds) != 3 {
		t.Fatalf("commands = %v", cmds)
	}
	wantTar := "tar -xzf " + up.path + " -C /home/user/workspace"
	if !strings.HasSuffix(cmds[0], "mkdir -p /home/user/workspace") || !strings.HasSuffix(cmds[1], wantTar) || !strings.HasSuffix(cmds[2], "rm -f "+up.path) {
		t.Fatalf("commands = %v", cmds)
	}
	for _, s := range envd.starts {
		if s.User != testUser {
			t.Fatalf("command ran as %q", s.User)
		}
	}
}

func TestSyncWorkspaceFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(*fakeEnvd)
		wantErrHas string
		wantRemove bool
	}{
		{
			name:       "upload fails",
			setup:      func(e *fakeEnvd) { e.uploadErr = errors.New("disk full") },
			wantErrHas: "upload workspace: disk full",
			wantRemove: true,
		},
		{
			name: "tar fails with its stderr",
			setup: func(e *fakeEnvd) {
				e.script = func(req e2b.StartRequest) *fakeProc {
					if slices.Contains(req.Args, "-xzf") {
						return output("", "tar: bad archive\n", 2)
					}
					return exited(0)
				}
			},
			wantErrHas: "extract workspace: exit status 2: tar: bad archive",
			wantRemove: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, _, envd := newProvider(nil, 0)
			tt.setup(envd)
			err := p.SyncWorkspace(context.Background(), testBox, strings.NewReader("x"))
			if err == nil || !strings.Contains(err.Error(), tt.wantErrHas) {
				t.Fatalf("err = %v, want %q", err, tt.wantErrHas)
			}
			if _, isExit := errors.AsType[*sandbox.ExitError](err); isExit {
				t.Fatalf("err = %v wraps *sandbox.ExitError, the CLI would exit without a message", err)
			}
			removed := slices.ContainsFunc(envd.argvs(), func(c string) bool { return strings.Contains(c, "rm -f /tmp/kaxa-sync-") })
			if removed != tt.wantRemove {
				t.Fatalf("temp archive removed = %v, want %v (commands %v)", removed, tt.wantRemove, envd.argvs())
			}
		})
	}
}

func TestSyncWorkspaceRejectsNilArchive(t *testing.T) {
	t.Parallel()

	p, _, envd := newProvider(nil, 0)
	if err := p.SyncWorkspace(context.Background(), testBox, nil); err == nil || len(envd.starts) != 0 {
		t.Fatalf("err = %v starts = %d", err, len(envd.starts))
	}
}

func TestRepoExists(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		repo    string
		code    int
		want    bool
		wantErr bool
	}{
		{name: "present", repo: "api", code: 0, want: true},
		{name: "absent", repo: "api", code: 1},
		{name: "test(1) failed", repo: "api", code: 2, wantErr: true},
		{name: "bad repo", repo: "../x", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, _, envd := newProvider(nil, 0)
			envd.script = func(e2b.StartRequest) *fakeProc { return exited(tt.code) }
			got, err := p.RepoExists(context.Background(), testBox, tt.repo)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("got = %v err = %v", got, err)
			}
			if _, isExit := errors.AsType[*sandbox.ExitError](err); isExit {
				t.Fatalf("err = %v wraps *sandbox.ExitError", err)
			}
			if !tt.wantErr {
				if cmd := envd.argvs()[0]; cmd != "test -d /home/user/workspace/api" {
					t.Fatalf("command = %q", cmd)
				}
			}
		})
	}
}

func TestSetupProjectsRunsInRepoAsUser(t *testing.T) {
	t.Parallel()

	p, api, envd := newProvider([]string{testKaxa, sandbox.SyncLabel}, 0)

	err := p.SetupProjects(context.Background(), testBox, "api", []project.Step{{
		Kind: project.KindGo, Commands: [][]string{{"go", "build", "./..."}},
	}})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if len(api.connect) != 1 {
		t.Fatalf("connect calls = %d, want the sandbox resumed once for the whole setup", len(api.connect))
	}
	var built bool
	for _, s := range envd.starts {
		if s.User != testUser {
			t.Fatalf("command ran as %q: %+v", s.User, s)
		}
		if slices.Contains(s.Args, "download") {
			built = true
			if s.Cwd != "/home/user/workspace/api" {
				t.Fatalf("download ran in %q", s.Cwd)
			}
		}
	}
	if !built {
		t.Fatalf("project command did not run: %v", envd.argvs())
	}
}

func TestSetupProjectsBuildsOnCustomTemplate(t *testing.T) {
	t.Parallel()

	p, api, envd := newProvider([]string{testKaxa, sandbox.SyncLabel}, 0)
	api.boxes[testBox].Template = testBig

	err := p.SetupProjects(context.Background(), testBox, "api", []project.Step{{
		Kind: project.KindGo, Commands: [][]string{{"go", "build", "./..."}},
	}})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	var built bool
	for _, s := range envd.starts {
		if slices.Contains(s.Args, "download") {
			t.Fatalf("a custom template was limited to downloads: %v", envd.argvs())
		}
		built = built || slices.Contains(s.Args, "build")
	}
	if !built {
		t.Fatalf("go build did not run: %v", envd.argvs())
	}
}

func TestSetupProjectsFailureCarriesStatus(t *testing.T) {
	t.Parallel()

	p, _, envd := newProvider(nil, 0)
	envd.script = func(e2b.StartRequest) *fakeProc { return output("", "", 101) }

	err := p.SetupProjects(context.Background(), testBox, "api", []project.Step{{
		Kind: project.KindRust, Commands: [][]string{{"cargo", "build"}},
	}})
	if err == nil || !strings.Contains(err.Error(), "exit status 101") {
		t.Fatalf("err = %v", err)
	}
	if _, isExit := errors.AsType[*sandbox.ExitError](err); isExit {
		t.Fatalf("err = %v wraps *sandbox.ExitError", err)
	}
}

func TestSetupProjectsRejectsBadRepoAndSkipsEmpty(t *testing.T) {
	t.Parallel()

	p, api, envd := newProvider(nil, 0)
	steps := []project.Step{{Kind: project.KindGo}}
	if err := p.SetupProjects(context.Background(), testBox, "..", steps); err == nil {
		t.Fatal("bad repo accepted")
	}
	if err := p.SetupProjects(context.Background(), testBox, "api", nil); err != nil {
		t.Fatalf("empty setup: %v", err)
	}
	if len(envd.starts) != 0 || len(api.connect) != 0 {
		t.Fatalf("starts = %d connects = %d, want none", len(envd.starts), len(api.connect))
	}
}
