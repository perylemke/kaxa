package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/cli"
	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/reposync"
	"github.com/perylemke/kaxa/internal/sandbox"
)

const (
	testSyncFlag = "--sync"
	testDirFlag  = "--dir"
)

func TestCreateSyncWithoutGitDoesNotCreate(t *testing.T) {
	t.Chdir(t.TempDir())

	fp := &fakeProvider{name: testSpritesName, createID: testBoxName}
	cmd := newCreateCmd(t, fp)
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdCreate, testBoxName, testSyncFlag})

	err := cmd.Execute()
	if !errors.Is(err, reposync.ErrNotRepository) {
		t.Fatalf("err = %v", err)
	}
	if fp.lastCreate.Name != "" {
		t.Fatalf("created %q", fp.lastCreate.Name)
	}
}

func TestCreateSyncRejectsProviderWithoutSync(t *testing.T) {
	initGitRepo(t)

	fp := &fakeProvider{name: testSpritesName, createID: testBoxName}
	cmd := newCreateCmd(t, fp)
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdCreate, testBoxName, testSyncFlag})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "sync is not available for") {
		t.Fatalf("err = %v", err)
	}
	if fp.lastCreate.Name != "" {
		t.Fatalf("created %q", fp.lastCreate.Name)
	}
}

func TestCreateSyncFailureDestroysSandbox(t *testing.T) {
	initGitRepo(t)

	opened := false
	restore := cli.SetOpenBrowserForTest(func(string) error {
		opened = true
		return nil
	})
	t.Cleanup(restore)

	fp := &syncingProvider{
		fakeProvider: &fakeProvider{
			name:     testSpritesName,
			createID: testBoxName,
			detail:   sandbox.Detail{ID: testBoxName, URL: "https://example.sprites.app"},
		},
		syncErr: errors.New("copy failed"),
	}
	cmd := newCreateCmd(t, fp)
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdCreate, testBoxName, testSyncFlag, testRunningFlag})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "copy failed") {
		t.Fatalf("err = %v", err)
	}
	if fp.destroyed != testBoxName {
		t.Fatalf("destroyed = %q", fp.destroyed)
	}
	if opened {
		t.Fatal("browser opened after a failed sync")
	}
}

func TestCreateSyncSuccessOpensBrowserAfterCopy(t *testing.T) {
	initGitRepo(t)

	const spriteURL = "https://example.sprites.app"
	fp := &syncingProvider{
		fakeProvider: &fakeProvider{
			name:     testSpritesName,
			createID: testBoxName,
			detail:   sandbox.Detail{ID: testBoxName, URL: spriteURL},
		},
	}
	var opened string
	restore := cli.SetOpenBrowserForTest(func(url string) error {
		if !fp.synced {
			t.Error("browser opened before sync")
		}
		opened = url
		return nil
	})
	t.Cleanup(restore)

	cmd := newCreateCmd(t, fp)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdCreate, testBoxName, testSyncFlag, testRunningFlag})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !fp.synced {
		t.Fatal("workspace was not synced")
	}
	if !slices.Contains(fp.lastCreate.Labels, sandbox.SyncLabel) {
		t.Fatalf("labels = %v", fp.lastCreate.Labels)
	}
	if fp.destroyed != "" {
		t.Fatalf("destroyed = %q", fp.destroyed)
	}
	if opened != spriteURL || !strings.Contains(out.String(), spriteURL) {
		t.Fatalf("opened = %q output = %q", opened, out.String())
	}
}

func TestCreateSyncInstallsDetectedProject(t *testing.T) {
	initGitRepo(t)
	writeGoMod(t)

	fp := &syncingProvider{
		fakeProvider: &fakeProvider{name: testSpritesName, createID: testBoxName},
	}
	cmd := newCreateCmd(t, fp)
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdCreate, testBoxName, testSyncFlag})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if fp.setupRepo == "" || len(fp.setupSteps) != 1 || fp.setupSteps[0].Kind != project.KindGo {
		t.Fatalf("setup repo = %q steps = %#v", fp.setupRepo, fp.setupSteps)
	}
	if fp.destroyed != "" {
		t.Fatalf("destroyed = %q", fp.destroyed)
	}
}

func TestCreateSyncInstallFailureDestroysSandbox(t *testing.T) {
	initGitRepo(t)
	writeGoMod(t)

	opened := false
	restore := cli.SetOpenBrowserForTest(func(string) error {
		opened = true
		return nil
	})
	t.Cleanup(restore)

	fp := &syncingProvider{
		fakeProvider: &fakeProvider{
			name:     testSpritesName,
			createID: testBoxName,
			detail:   sandbox.Detail{ID: testBoxName, URL: "https://example.sprites.app"},
		},
		setupErr: errors.New("go build failed"),
	}
	cmd := newCreateCmd(t, fp)
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdCreate, testBoxName, testSyncFlag, testRunningFlag})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "go build failed") {
		t.Fatalf("err = %v", err)
	}
	if fp.destroyed != testBoxName {
		t.Fatalf("destroyed = %q", fp.destroyed)
	}
	if opened {
		t.Fatal("browser opened after a failed install")
	}
}

func TestCreateSyncDirCopiesNamedRepo(t *testing.T) {
	repo := initGitRepo(t)
	writeGoMod(t)
	if err := os.Mkdir("nested", 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	other := t.TempDir()
	t.Chdir(other)
	rel, err := filepath.Rel(other, filepath.Join(repo, "nested"))
	if err != nil {
		t.Fatalf("rel: %v", err)
	}

	fp := &syncingProvider{
		fakeProvider: &fakeProvider{name: testSpritesName, createID: testBoxName},
	}
	cmd := newCreateCmd(t, fp)
	cmd.SetArgs([]string{
		testProviderFlag, testSpritesName, testCmdCreate, testBoxName,
		testSyncFlag, testDirFlag, rel,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !fp.synced || fp.setupRepo != filepath.Base(repo) {
		t.Fatalf("synced=%v repo=%q", fp.synced, fp.setupRepo)
	}
	if len(fp.setupSteps) != 1 || fp.setupSteps[0].Kind != project.KindGo {
		t.Fatalf("steps = %#v", fp.setupSteps)
	}
}

func TestCreateSyncDirRejectsProviderWithoutSync(t *testing.T) {
	repo := initGitRepo(t)
	t.Chdir(t.TempDir())

	fp := &fakeProvider{name: testSpritesName, createID: testBoxName}
	cmd := newCreateCmd(t, fp)
	cmd.SetArgs([]string{
		testProviderFlag, testSpritesName, testCmdCreate, testBoxName,
		testSyncFlag, testDirFlag, repo,
	})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "sync is not available for") {
		t.Fatalf("err = %v", err)
	}
	if fp.lastCreate.Name != "" {
		t.Fatalf("created %q", fp.lastCreate.Name)
	}
}

func TestCreateSyncDirRequiresSync(t *testing.T) {
	fp := &fakeProvider{name: testSpritesName, createID: testBoxName}
	cmd := newCreateCmd(t, fp)
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdCreate, testBoxName, testDirFlag, t.TempDir()})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--dir requires --sync") {
		t.Fatalf("err = %v", err)
	}
	if fp.lastCreate.Name != "" {
		t.Fatalf("created %q", fp.lastCreate.Name)
	}
}

func TestCreateSyncDirRejectsBadPath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	plain := t.TempDir()
	tests := []struct {
		name string
		dir  string
		want string
	}{
		{name: "missing", dir: filepath.Join(dir, "missing"), want: "was not found"},
		{name: "file", dir: file, want: "not a directory"},
		{name: "empty", dir: " ", want: "directory is required"},
		{name: "not a repo", dir: plain, want: plain + " is not inside a git repository"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp := &fakeProvider{name: testSpritesName, createID: testBoxName}
			cmd := newCreateCmd(t, fp)
			cmd.SetArgs([]string{
				testProviderFlag, testSpritesName, testCmdCreate, testBoxName,
				testSyncFlag, testDirFlag, tt.dir,
			})

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

type syncingProvider struct {
	*fakeProvider
	syncErr    error
	synced     bool
	setupErr   error
	setupRepo  string
	setupSteps []project.Step
}

func (s *syncingProvider) SyncWorkspace(_ context.Context, _ sandbox.ID, archive io.Reader) error {
	if archive != nil {
		if _, err := io.Copy(io.Discard, io.LimitReader(archive, 8<<20)); err != nil {
			return err
		}
	}
	s.synced = true
	return s.syncErr
}

func (s *syncingProvider) SetupProjects(_ context.Context, _ sandbox.ID, repoName string, steps []project.Step) error {
	if !s.synced {
		return errors.New("setup before sync")
	}
	s.setupRepo = repoName
	s.setupSteps = steps
	return s.setupErr
}

func writeGoMod(t *testing.T) {
	t.Helper()
	if err := os.WriteFile("go.mod", []byte("module example.com/demo\n\ngo 1.27\n"), 0o600); err != nil {
		t.Fatalf("go.mod: %v", err)
	}
}

func writeAPIPyproject(t *testing.T) {
	t.Helper()
	body := "[project]\nname = \"demo\"\ndependencies = [\"fastapi>=0.115\"]\n"
	if err := os.WriteFile("pyproject.toml", []byte(body), 0o600); err != nil {
		t.Fatalf("pyproject: %v", err)
	}
}

func newCreateCmd(t *testing.T, p sandbox.Provider) *cobra.Command {
	t.Helper()
	reg := spritesRegistry(t, p)
	cmd := cli.NewRootCmd(reg)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	t.Setenv("KAXA_SPRITES_TOKEN", "tok")
	return cmd
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	t.Chdir(dir)
	return dir
}
