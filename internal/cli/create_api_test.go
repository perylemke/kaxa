package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/cli"
	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/sandbox"
)

const (
	testAPIFlag    = "--api"
	testAPICmdFlag = "--api-cmd"
	testAPIEnvFlag = "--api-env"
	testUvicorn    = "uvicorn"
)

func TestCreateAPIFlagValidation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing command",
			args: []string{testAPIFlag},
			want: "api command is required",
		},
		{
			name: "command without api",
			args: []string{testAPICmdFlag, testUvicorn},
			want: "--api-cmd requires --api",
		},
		{
			name: "port without api",
			args: []string{"--api-port", "9000"},
			want: "--api-port requires --api",
		},
		{
			name: "env without api",
			args: []string{testAPIEnvFlag, "MODE=prod"},
			want: "--api-env requires --api",
		},
		{
			name: "reserved env",
			args: []string{testAPIFlag, testAPICmdFlag, testUvicorn, testAPIEnvFlag, "KAXA_SPRITES_TOKEN"},
			want: "reserved",
		},
		{
			name: "unset inherited env",
			args: []string{testAPIFlag, testAPICmdFlag, testUvicorn, testAPIEnvFlag, "SAMPLE_TEST_UNSET_VAR_X"},
			want: "SAMPLE_TEST_UNSET_VAR_X is not set",
		},
		{
			name: "empty argument",
			args: []string{testAPIFlag, testAPICmdFlag, "uvicorn,,app"},
			want: "empty argument",
		},
		{
			name: "public without support",
			args: []string{"--public"},
			want: "public urls is not available for",
		},
		{
			name: "tmux port",
			args: []string{testRunningFlag, testAPIFlag, "--api-port", "8080", testAPICmdFlag, testUvicorn},
			want: "8080",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := &fakeProvider{name: testSpritesName, createID: testBoxName}
			cmd := newCreateCmd(t, fp)
			cmd.SetArgs(append([]string{testProviderFlag, testSpritesName, testCmdCreate, testBoxName}, tt.args...))

			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v", err)
			}
			if fp.lastCreate.Name != "" {
				t.Fatalf("created %q", fp.lastCreate.Name)
			}
		})
	}
}

func TestCreateAPIRejectsProviderWithoutPublish(t *testing.T) {
	fp := &fakeProvider{name: testSpritesName, createID: testBoxName}
	cmd := newCreateCmd(t, fp)
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdCreate, testBoxName, testAPIFlag, testAPICmdFlag, testUvicorn})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "api publish is not available for") {
		t.Fatalf("err = %v", err)
	}
	if fp.lastCreate.Name != "" {
		t.Fatalf("created %q", fp.lastCreate.Name)
	}
}

func TestCreateAPIPublishesService(t *testing.T) {
	const spriteURL = "https://example.sprites.app"
	fp := &publishingProvider{
		fakeProvider: &fakeProvider{
			name:     testSpritesName,
			createID: testBoxName,
			detail:   sandbox.Detail{ID: testBoxName, URL: spriteURL},
		},
	}
	opened := false
	restore := cli.SetOpenBrowserForTest(func(string) error {
		opened = true
		return nil
	})
	t.Cleanup(restore)

	cmd := newCreateCmd(t, fp)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetArgs([]string{
		testProviderFlag, testSpritesName, testCmdCreate, testBoxName,
		testAPIFlag, testAPICmdFlag, ".venv/bin/uvicorn,app.main:app,--host,0.0.0.0,--port,8000",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if fp.workDir != "/home/sprite" || fp.port != 8000 || len(fp.argv) != 6 || fp.argv[0] != ".venv/bin/uvicorn" {
		t.Fatalf("publish dir=%q port=%d argv=%#v", fp.workDir, fp.port, fp.argv)
	}
	if fp.destroyed != "" || fp.madePublic || opened {
		t.Fatalf("destroyed=%q public=%v opened=%v", fp.destroyed, fp.madePublic, opened)
	}
	got := out.String()
	if !strings.Contains(got, spriteURL) || !strings.Contains(got, "port 8000") || !strings.Contains(got, "Sprites account") {
		t.Fatalf("output = %q", got)
	}
}

func TestCreateAPIPassesEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://db:5432/app")
	fp := &publishingProvider{
		fakeProvider: &fakeProvider{name: testSpritesName, createID: testBoxName},
	}
	cmd := newCreateCmd(t, fp)
	cmd.SetArgs([]string{
		testProviderFlag, testSpritesName, testCmdCreate, testBoxName,
		testAPIFlag, testAPICmdFlag, testUvicorn,
		testAPIEnvFlag, "DATABASE_URL", testAPIEnvFlag, "MODE=prod",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	want := []string{"DATABASE_URL=postgres://db:5432/app", "MODE=prod"}
	if !slices.Equal(fp.env, want) {
		t.Fatalf("env = %#v", fp.env)
	}
}

func TestCreateAPIWithSyncUsesRepoDir(t *testing.T) {
	initGitRepo(t)
	writeAPIPyproject(t)
	fp := &publishingProvider{
		fakeProvider: &fakeProvider{name: testSpritesName, createID: testBoxName},
	}
	cmd := newCreateCmd(t, fp)
	cmd.SetArgs([]string{
		testProviderFlag, testSpritesName, testCmdCreate, testBoxName,
		testSyncFlag, testAPIFlag, testAPICmdFlag, testUvicorn,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.HasPrefix(fp.workDir, "/home/sprite/workspace/") || strings.Contains(fp.workDir, "..") {
		t.Fatalf("workDir = %q", fp.workDir)
	}
}

func TestCreateAPIFailureDestroysSandbox(t *testing.T) {
	fp := &publishingProvider{
		fakeProvider: &fakeProvider{name: testSpritesName, createID: testBoxName},
		publishErr:   errors.New("service failed"),
	}
	cmd := newCreateCmd(t, fp)
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdCreate, testBoxName, testAPIFlag, testAPICmdFlag, testUvicorn})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "service failed") {
		t.Fatalf("err = %v", err)
	}
	if fp.destroyed != testBoxName {
		t.Fatalf("destroyed = %q", fp.destroyed)
	}
}

func TestCreatePublicFailureKeepsSandbox(t *testing.T) {
	fp := &publishingProvider{
		fakeProvider: &fakeProvider{
			name:     testSpritesName,
			createID: testBoxName,
			detail:   sandbox.Detail{ID: testBoxName, URL: "https://example.sprites.app"},
		},
		publicErr: errors.New("auth failed"),
	}
	cmd := newCreateCmd(t, fp)
	cmd.SetArgs([]string{
		testProviderFlag, testSpritesName, testCmdCreate, testBoxName,
		testAPIFlag, "--public", testAPICmdFlag, testUvicorn,
	})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "auth failed") {
		t.Fatalf("err = %v", err)
	}
	if fp.destroyed != "" || !fp.published {
		t.Fatalf("destroyed=%q published=%v", fp.destroyed, fp.published)
	}
}

func TestCreateAPISyncRejectsCLI(t *testing.T) {
	initGitRepo(t)
	writeGoMod(t)
	fp := &publishingProvider{
		fakeProvider: &fakeProvider{name: testSpritesName, createID: testBoxName},
	}
	cmd := newCreateCmd(t, fp)
	cmd.SetArgs([]string{
		testProviderFlag, testSpritesName, testCmdCreate, testBoxName,
		testSyncFlag, testAPIFlag, testAPICmdFlag, testUvicorn,
	})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "not an HTTP API") {
		t.Fatalf("err = %v", err)
	}
	if fp.lastCreate.Name != "" || fp.published {
		t.Fatalf("created %q published %v", fp.lastCreate.Name, fp.published)
	}
}

type publishingProvider struct {
	*fakeProvider
	publishErr error
	publicErr  error
	published  bool
	madePublic bool
	workDir    string
	port       int
	argv       []string
	env        []string
}

func (p *publishingProvider) SyncWorkspace(_ context.Context, _ sandbox.ID, archive io.Reader) error {
	if archive == nil {
		return nil
	}
	_, err := io.Copy(io.Discard, io.LimitReader(archive, 8<<20))
	return err
}

func (p *publishingProvider) HomeDir() string { return "/home/sprite" }

func (p *publishingProvider) RepoDir(repo string) (string, error) {
	return "/home/sprite/workspace/" + repo, nil
}

func (p *publishingProvider) SetupProjects(context.Context, sandbox.ID, string, []project.Step) error {
	return nil
}

func (p *publishingProvider) PublishAPI(_ context.Context, _ sandbox.ID, workDir string, port int, argv, env []string) error {
	p.env = append([]string(nil), env...)
	p.published = true
	p.workDir = workDir
	p.port = port
	p.argv = append([]string(nil), argv...)
	return p.publishErr
}

func (p *publishingProvider) SetURLAuth(_ context.Context, _ sandbox.ID, auth string) error {
	if !p.published {
		return errors.New("public before publish")
	}
	if auth != sandbox.URLAuthPublic {
		return errors.New("unexpected url auth " + auth)
	}
	p.madePublic = true
	return p.publicErr
}
