package sprites

import (
	"context"
	"io"
	"slices"
	"testing"

	"github.com/perylemke/kaxa/internal/project"
)

const projectBuild = "build"

type remoteCall struct {
	dir  string
	env  []string
	argv []string
}

// recordingClient records RunIn calls; every other method panics (nil embed).
type recordingClient struct {
	sdkClient
	calls []remoteCall
}

func (r *recordingClient) RunIn(_ context.Context, _, dir string, env, argv []string, _ io.Reader, _, _ io.Writer) error {
	r.calls = append(r.calls, remoteCall{dir: dir, env: slices.Clone(env), argv: slices.Clone(argv)})
	return nil
}

func TestSetupProjectsRunsStepsInWorkspaceRepo(t *testing.T) {
	t.Parallel()

	client := &recordingClient{}
	p := NewWithClient(client, "token")
	err := p.SetupProjects(context.Background(), "box", "api", []project.Step{{
		Kind:     project.KindGo,
		Commands: [][]string{{"go", projectBuild, "./..."}},
	}})
	if err != nil {
		t.Fatalf("SetupProjects: %v", err)
	}
	var build remoteCall
	for _, call := range client.calls {
		if len(call.argv) == 3 && call.argv[1] == projectBuild {
			build = call
		}
	}
	if build.dir != workspaceDir+"/api" || build.argv[0] != "go" {
		t.Fatalf("build = %#v, calls = %#v", build, client.calls)
	}
}

func TestSetupProjectsRejectsRepoDir(t *testing.T) {
	t.Parallel()

	client := &recordingClient{}
	p := NewWithClient(client, "token")
	if err := p.SetupProjects(context.Background(), "box", "..", nil); err == nil {
		t.Fatal("expected invalid directory error")
	}
	if len(client.calls) != 0 {
		t.Fatalf("calls = %#v", client.calls)
	}
}

func TestSpriteLayoutEnv(t *testing.T) {
	t.Parallel()

	env := spriteLayout.Env()
	if len(env) != 3 || env[0] != "HOME=/home/sprite" || env[1] != "USER=sprite" {
		t.Fatalf("env = %v", env)
	}
}
