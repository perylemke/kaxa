package mcp

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/config"
	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
)

// fakeProvider implements every optional interface.
type fakeProvider struct {
	name        string
	unavailable bool

	created   []sandbox.Spec
	destroyed []sandbox.ID
	synced    bool
	published bool
	pubDir    string
	pubPort   int
	pubEnv    []string
	auth      string

	execFn    func(sandbox.ExecOptions) error
	createErr error
	syncErr   error
	detail    sandbox.Detail
	infos     []sandbox.Info
}

func (f *fakeProvider) Name() string           { return f.name }
func (f *fakeProvider) Available() bool        { return !f.unavailable }
func (f *fakeProvider) HomeDir() string        { return "/home/box" }
func (f *fakeProvider) Configure(string) error { return nil }

func (f *fakeProvider) RepoDir(repo string) (string, error) { return "/home/box/ws/" + repo, nil }

func (f *fakeProvider) Create(_ context.Context, spec sandbox.Spec) (sandbox.ID, error) {
	if f.createErr != nil {
		return "", f.createErr
	}
	f.created = append(f.created, spec)
	return sandbox.ID(spec.Name), nil
}

func (f *fakeProvider) List(context.Context) ([]sandbox.Info, error) { return f.infos, nil }

func (f *fakeProvider) Inspect(_ context.Context, id sandbox.ID) (sandbox.Detail, error) {
	if f.detail.ID != "" {
		return f.detail, nil
	}
	return sandbox.Detail{ID: id, Provider: f.name, Status: testWarm}, nil
}

func (f *fakeProvider) Restart(context.Context, sandbox.ID) error { return nil }
func (f *fakeProvider) Upgrade(context.Context, sandbox.ID) error { return nil }

func (f *fakeProvider) Exec(_ context.Context, _ sandbox.ID, opts sandbox.ExecOptions) error {
	if f.execFn != nil {
		return f.execFn(opts)
	}
	return nil
}

func (f *fakeProvider) Destroy(_ context.Context, id sandbox.ID) error {
	f.destroyed = append(f.destroyed, id)
	return nil
}

func (f *fakeProvider) SyncWorkspace(_ context.Context, _ sandbox.ID, archive io.Reader) error {
	if archive != nil {
		if _, err := io.Copy(io.Discard, archive); err != nil {
			return err
		}
	}
	f.synced = true
	return f.syncErr
}

func (f *fakeProvider) SetupProjects(context.Context, sandbox.ID, string, []project.Step) error {
	return nil
}

func (f *fakeProvider) PublishAPI(_ context.Context, _ sandbox.ID, workDir string, port int, _, env []string) error {
	f.published, f.pubDir, f.pubPort = true, workDir, port
	f.pubEnv = append([]string(nil), env...)
	return nil
}

func (f *fakeProvider) SetURLAuth(_ context.Context, _ sandbox.ID, auth string) error {
	f.auth = auth
	return nil
}

// plainProvider has no optional interfaces.
type plainProvider struct{ name string }

func (p plainProvider) Name() string { return p.name }
func (plainProvider) Create(_ context.Context, spec sandbox.Spec) (sandbox.ID, error) {
	return sandbox.ID(spec.Name), nil
}
func (plainProvider) List(context.Context) ([]sandbox.Info, error) { return nil, nil }
func (plainProvider) Inspect(context.Context, sandbox.ID) (sandbox.Detail, error) {
	return sandbox.Detail{}, nil
}
func (plainProvider) Restart(context.Context, sandbox.ID) error { return nil }
func (plainProvider) Upgrade(context.Context, sandbox.ID) error { return nil }
func (plainProvider) Exec(context.Context, sandbox.ID, sandbox.ExecOptions) error {
	return nil
}
func (plainProvider) Destroy(context.Context, sandbox.ID) error { return nil }

func newTestService(t *testing.T, root, defaultProvider string, providers ...sandbox.Provider) *service {
	t.Helper()
	reg := registry.New()
	for _, p := range providers {
		if err := reg.Register(p); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	svc, err := newService(reg, config.Config{Provider: defaultProvider}, Options{Root: root, Stderr: io.Discard})
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	return svc
}

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}

func wantErr(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), substr) {
		t.Fatalf("err = %v, want %q", err, substr)
	}
}

func TestListProvidersReportsAvailabilityAndCapabilities(t *testing.T) {
	t.Parallel()

	svc := newTestService(t, t.TempDir(), testSprites,
		&fakeProvider{name: testSprites},
		&fakeProvider{name: testModal, unavailable: true},
		plainProvider{name: "plain"},
	)
	out, err := svc.listProviders(context.Background(), listProvidersIn{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out.Providers) != 3 {
		t.Fatalf("providers = %+v", out.Providers)
	}
	byName := map[string]providerInfo{}
	for _, p := range out.Providers {
		byName[p.Name] = p
	}
	sprites := byName[testSprites]
	if !sprites.Available || !sprites.Default || !sprites.Capabilities.API || !sprites.Capabilities.PublicURL || !sprites.Capabilities.Sync {
		t.Fatalf("sprites = %+v", sprites)
	}
	if modal := byName[testModal]; modal.Available || modal.Default {
		t.Fatalf("modal = %+v", modal)
	}
	if plain := byName["plain"]; !plain.Available || plain.Capabilities != (sandbox.Capabilities{}) {
		t.Fatalf("plain = %+v", plain)
	}
}

func TestListProvidersWorksWithoutDefault(t *testing.T) {
	t.Parallel()

	svc := newTestService(t, t.TempDir(), "", &fakeProvider{name: testSprites})
	out, err := svc.listProviders(context.Background(), listProvidersIn{})
	if err != nil || len(out.Providers) != 1 || out.Providers[0].Default {
		t.Fatalf("out = %+v err = %v", out, err)
	}
}

func TestProviderSelection(t *testing.T) {
	t.Parallel()

	sprites := &fakeProvider{name: testSprites}
	modal := &fakeProvider{name: testModal, unavailable: true}
	svc := newTestService(t, t.TempDir(), testSprites, sprites, modal)

	if p, err := svc.provider(""); err != nil || p.Name() != testSprites {
		t.Fatalf("default: %v %v", p, err)
	}
	_, err := svc.provider(testModal)
	wantErr(t, err, "modal provider is not available yet")
	_, err = svc.provider("nope")
	wantErr(t, err, `unknown provider "nope"`)

	noDefault := newTestService(t, t.TempDir(), "", sprites)
	_, err = noDefault.provider("")
	wantErr(t, err, "missing sandbox provider")
}

func TestCreateWithoutSync(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testSprites, detail: sandbox.Detail{ID: testBox, URL: testURL, Auth: sandbox.URLAuthSprite}}
	svc := newTestService(t, t.TempDir(), testSprites, fp)

	out, err := svc.create(context.Background(), createIn{Name: testBox, Running: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if out.ID != testBox || out.Provider != testSprites || out.WorkDir != "" || out.URL != testURL || out.Auth != sandbox.URLAuthSprite {
		t.Fatalf("out = %+v", out)
	}
	if len(fp.created) != 1 || !fp.created[0].Running || !slices.Contains(fp.created[0].Labels, "kaxa") {
		t.Fatalf("spec = %+v", fp.created)
	}
}

func TestCreateRejectsBeforeCreating(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   createIn
		want string
	}{
		{name: "no name", in: createIn{}, want: "name is required"},
		{name: "dir without sync", in: createIn{Name: testBox, Dir: "/tmp"}, want: "dir requires sync"},
		{name: "api_cmd without api", in: createIn{Name: testBox, APICmd: []string{"uvicorn"}}, want: "api_cmd requires api"},
		{name: "api_port without api", in: createIn{Name: testBox, APIPort: 9000}, want: "api_port requires api"},
		{name: "api_env without api", in: createIn{Name: testBox, APIEnv: []string{"A=b"}}, want: "api_env requires api"},
		{name: "api_env reserved", in: createIn{Name: testBox, API: true, APICmd: []string{"x"}, APIEnv: []string{"KAXA_SPRITES_TOKEN"}}, want: "reserved"},
		{name: "api without command", in: createIn{Name: testBox, API: true}, want: "api command is required"},
		{name: "tmux port", in: createIn{Name: testBox, API: true, Running: true, APIPort: 8080, APICmd: []string{"x"}}, want: "8080"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fp := &fakeProvider{name: testSprites}
			svc := newTestService(t, t.TempDir(), testSprites, fp)
			_, err := svc.create(context.Background(), tt.in)
			wantErr(t, err, tt.want)
			if len(fp.created) != 0 {
				t.Fatalf("created %v", fp.created)
			}
		})
	}
}

func TestCreateRejectsUnavailableProvider(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testModal, unavailable: true}
	svc := newTestService(t, t.TempDir(), testModal, fp)
	_, err := svc.create(context.Background(), createIn{Name: testBox})
	wantErr(t, err, "not available yet")
	if len(fp.created) != 0 {
		t.Fatalf("created %v", fp.created)
	}
}

func TestCreateRejectsProviderWithoutCapability(t *testing.T) {
	t.Parallel()

	svc := newTestService(t, t.TempDir(), "plain", plainProvider{name: "plain"})
	_, err := svc.create(context.Background(), createIn{Name: testBox, Public: true})
	wantErr(t, err, "plain provider does not support public urls")
}

func TestCreateSyncReturnsWorkDir(t *testing.T) {
	t.Parallel()

	repo := gitRepo(t)
	fp := &fakeProvider{name: testSprites}
	svc := newTestService(t, repo, testSprites, fp)

	out, err := svc.create(context.Background(), createIn{Name: testBox, Sync: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !fp.synced || out.WorkDir != "/home/box/ws/"+filepath.Base(repo) {
		t.Fatalf("synced=%v out=%+v", fp.synced, out)
	}
	if !slices.Contains(fp.created[0].Labels, sandbox.SyncLabel) {
		t.Fatalf("labels = %v", fp.created[0].Labels)
	}
}

func TestCreateSyncFailureDestroysSandbox(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testSprites, syncErr: errors.New("copy failed")}
	svc := newTestService(t, gitRepo(t), testSprites, fp)

	_, err := svc.create(context.Background(), createIn{Name: testBox, Sync: true})
	wantErr(t, err, "copy failed")
	if !slices.Equal(fp.destroyed, []sandbox.ID{testBox}) {
		t.Fatalf("destroyed = %v", fp.destroyed)
	}
}

func TestCreateSyncDirInsideServerRoot(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	repo := filepath.Join(parent, "app")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	fp := &fakeProvider{name: testSprites}
	svc := newTestService(t, parent, testSprites, fp)

	out, err := svc.create(context.Background(), createIn{Name: testBox, Sync: true, Dir: repo})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if out.WorkDir != "/home/box/ws/app" {
		t.Fatalf("work dir = %q", out.WorkDir)
	}
}

func TestCreateSyncRejectsRepoOutsideServerRoot(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testSprites}
	svc := newTestService(t, gitRepo(t), testSprites, fp)
	other := gitRepo(t)

	_, err := svc.create(context.Background(), createIn{Name: testBox, Sync: true, Dir: other})
	wantErr(t, err, "outside the server working directory")
	if len(fp.created) != 0 || fp.synced {
		t.Fatalf("created=%v synced=%v", fp.created, fp.synced)
	}
}

func TestCreateSyncFailsOutsideGitRepo(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testSprites}
	svc := newTestService(t, t.TempDir(), testSprites, fp)
	_, err := svc.create(context.Background(), createIn{Name: testBox, Sync: true})
	wantErr(t, err, "not inside a git repository")
	if len(fp.created) != 0 {
		t.Fatalf("created %v", fp.created)
	}
}

func TestCheckRepoScope(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	child := filepath.Join(base, "child")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	sibling := t.TempDir()
	link := filepath.Join(base, "link")
	if err := os.Symlink(sibling, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	tests := []struct {
		name  string
		repo  string
		scope string
		ok    bool
	}{
		{name: "same", repo: base, scope: base, ok: true},
		{name: "repo below scope", repo: child, scope: base, ok: true},
		{name: "repo above scope", repo: base, scope: child, ok: true},
		{name: "unrelated", repo: sibling, scope: base},
		{name: "symlink escapes scope", repo: link, scope: base},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := checkRepoScope(tt.repo, tt.scope)
			if tt.ok && err != nil {
				t.Fatalf("err = %v", err)
			}
			if !tt.ok && err == nil {
				t.Fatal("scope accepted")
			}
		})
	}
}

func TestExecReturnsExitCodeWithoutFailing(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testSprites, execFn: func(opts sandbox.ExecOptions) error {
		_, _ = io.WriteString(opts.Stdout, "out")
		_, _ = io.WriteString(opts.Stderr, "err")
		return &sandbox.ExitError{Code: 7}
	}}
	svc := newTestService(t, t.TempDir(), testSprites, fp)

	out, err := svc.exec(context.Background(), execIn{Name: testBox, Command: []string{"false"}})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if out.ExitCode != 7 || out.Stdout != "out" || out.Stderr != "err" || out.Truncated {
		t.Fatalf("out = %+v", out)
	}
}

func TestExecPassesArgvAndNoStdin(t *testing.T) {
	t.Parallel()

	var got sandbox.ExecOptions
	fp := &fakeProvider{name: testSprites, execFn: func(opts sandbox.ExecOptions) error {
		got = opts
		return nil
	}}
	svc := newTestService(t, t.TempDir(), testSprites, fp)

	if _, err := svc.exec(context.Background(), execIn{Name: testBox, Command: []string{"echo", "a b", ";rm"}}); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !slices.Equal(got.Command, []string{"echo", "a b", ";rm"}) || got.Stdin != nil || got.TTY {
		t.Fatalf("opts = %+v", got)
	}
}

func TestExecTruncatesLargeOutput(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testSprites, execFn: func(opts sandbox.ExecOptions) error {
		chunk := strings.Repeat("x", 64<<10)
		for range 6 {
			if _, err := io.WriteString(opts.Stdout, chunk); err != nil {
				return err
			}
		}
		return nil
	}}
	svc := newTestService(t, t.TempDir(), testSprites, fp)

	out, err := svc.exec(context.Background(), execIn{Name: testBox, Command: []string{"yes"}})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !out.Truncated || !strings.HasSuffix(out.Stdout, "[output truncated at 262144 bytes]") {
		t.Fatalf("truncated=%v tail=%q", out.Truncated, out.Stdout[len(out.Stdout)-40:])
	}
	if got := strings.Count(out.Stdout, "x"); got != maxOutputBytes {
		t.Fatalf("kept %d bytes, want %d", got, maxOutputBytes)
	}
	if out.Stderr != "" {
		t.Fatalf("stderr = %q", out.Stderr)
	}
}

func TestExecRejectsBadInput(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testSprites, execFn: func(sandbox.ExecOptions) error {
		t.Error("exec reached provider")
		return nil
	}}
	svc := newTestService(t, t.TempDir(), testSprites, fp)

	_, err := svc.exec(context.Background(), execIn{Name: testBox})
	wantErr(t, err, "command is required")
	_, err = svc.exec(context.Background(), execIn{Name: testBox, Command: []string{" "}})
	wantErr(t, err, "command is required")
	_, err = svc.exec(context.Background(), execIn{Command: []string{"ls"}})
	wantErr(t, err, "name is required")
}

func TestExecProviderFailureIsAnError(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testSprites, execFn: func(sandbox.ExecOptions) error {
		return errors.New("sandbox not found")
	}}
	svc := newTestService(t, t.TempDir(), testSprites, fp)

	_, err := svc.exec(context.Background(), execIn{Name: testNoBox, Command: []string{"ls"}})
	wantErr(t, err, "sandbox not found")
}

func TestListAndInspect(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	fp := &fakeProvider{
		name:  testSprites,
		infos: []sandbox.Info{{ID: "a", Provider: testSprites, Status: testWarm}},
		detail: sandbox.Detail{
			ID: testBox, Provider: testSprites, Status: testWarm, URL: testURL,
			Auth: sandbox.URLAuthPublic, Organization: "org", CreatedAt: created,
		},
	}
	svc := newTestService(t, t.TempDir(), testSprites, fp)
	ctx := context.Background()

	list, err := svc.listSandboxes(ctx, listIn{})
	if err != nil || len(list.Sandboxes) != 1 || list.Sandboxes[0].ID != "a" || list.Sandboxes[0].Labels == nil {
		t.Fatalf("list = %+v err = %v", list, err)
	}
	detail, err := svc.inspect(ctx, nameIn{Name: testBox})
	if err != nil || detail.CreatedAt != "2026-10-26T12:00:00Z" || detail.UpdatedAt != "" || detail.URL != testURL {
		t.Fatalf("inspect = %+v err = %v", detail, err)
	}
}

func TestActions(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testSprites}
	svc := newTestService(t, t.TempDir(), testSprites, fp)
	ctx := context.Background()

	for status, run := range map[string]func(context.Context, nameIn) (actionOut, error){
		"restarted": svc.restart, "upgraded": svc.upgrade, "destroyed": svc.destroy,
	} {
		out, err := run(ctx, nameIn{Name: testBox})
		if err != nil || out.Status != status || out.ID != testBox || out.Provider != testSprites {
			t.Fatalf("%s = %+v err = %v", status, out, err)
		}
	}
	if !slices.Equal(fp.destroyed, []sandbox.ID{testBox}) {
		t.Fatalf("destroyed = %v", fp.destroyed)
	}
	if _, err := svc.destroy(ctx, nameIn{}); err == nil {
		t.Fatal("destroy accepted an empty name")
	}
}

func TestPublishAPI(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testSprites}
	svc := newTestService(t, t.TempDir(), testSprites, fp)
	ctx := context.Background()

	out, err := svc.publishAPI(ctx, publishIn{Name: testBox, APICmd: []string{"uvicorn", "app:app"}})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if out.Status != "published" || fp.pubDir != "/home/box" || fp.pubPort != 8000 {
		t.Fatalf("out=%+v dir=%q port=%d", out, fp.pubDir, fp.pubPort)
	}

	_, err = svc.publishAPI(ctx, publishIn{Name: testBox, APICmd: []string{"x"}, WorkDir: "/home/box/ws/app/../app", APIPort: 9000})
	if err != nil || fp.pubDir != "/home/box/ws/app" || fp.pubPort != 9000 {
		t.Fatalf("err=%v dir=%q port=%d", err, fp.pubDir, fp.pubPort)
	}

	_, err = svc.publishAPI(ctx, publishIn{Name: testBox, APICmd: []string{"x"}, WorkDir: "relative"})
	wantErr(t, err, "absolute path")
	_, err = svc.publishAPI(ctx, publishIn{Name: testBox})
	wantErr(t, err, "api command is required")
}

func TestPublishAPIPassesEnv(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testSprites}
	svc := newTestService(t, t.TempDir(), testSprites, fp)
	ctx := context.Background()

	_, err := svc.publishAPI(ctx, publishIn{Name: testBox, APICmd: []string{"x"}, APIEnv: []string{"MODE=prod"}})
	if err != nil || !slices.Equal(fp.pubEnv, []string{"MODE=prod"}) {
		t.Fatalf("err=%v env=%v", err, fp.pubEnv)
	}
	_, err = svc.publishAPI(ctx, publishIn{Name: testBox, APICmd: []string{"x"}, APIEnv: []string{"PATH=/tmp"}})
	wantErr(t, err, "reserved")
}

func TestPublishAPIAndURLAuthNeedCapabilities(t *testing.T) {
	t.Parallel()

	svc := newTestService(t, t.TempDir(), "plain", plainProvider{name: "plain"})
	ctx := context.Background()

	_, err := svc.publishAPI(ctx, publishIn{Name: testBox, APICmd: []string{"x"}})
	wantErr(t, err, "plain provider does not support api publish")
	_, err = svc.setURLAuth(ctx, urlAuthIn{Name: testBox, Auth: sandbox.URLAuthPublic})
	wantErr(t, err, "plain provider does not support url auth")
}

func TestSetURLAuth(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testSprites, detail: sandbox.Detail{ID: testBox, URL: testURL}}
	svc := newTestService(t, t.TempDir(), testSprites, fp)
	ctx := context.Background()

	out, err := svc.setURLAuth(ctx, urlAuthIn{Name: testBox, Auth: sandbox.URLAuthPublic})
	if err != nil || fp.auth != sandbox.URLAuthPublic || out.URL != testURL || out.Auth != sandbox.URLAuthPublic {
		t.Fatalf("out=%+v err=%v auth=%q", out, err, fp.auth)
	}
	_, err = svc.setURLAuth(ctx, urlAuthIn{Name: testBox, Auth: "open"})
	wantErr(t, err, "invalid url auth")
}

func TestLimitedBuffer(t *testing.T) {
	t.Parallel()

	b := newLimitedBuffer(5)
	for _, s := range []string{"ab", "cdefg", "hi"} {
		n, err := b.Write([]byte(s))
		if err != nil || n != len(s) {
			t.Fatalf("write %q = %d, %v", s, n, err)
		}
	}
	if !b.wasTruncated() || !strings.HasPrefix(b.String(), "abcde\n[output truncated") {
		t.Fatalf("buffer = %q truncated = %v", b.String(), b.wasTruncated())
	}

	exact := newLimitedBuffer(3)
	_, _ = exact.Write([]byte("abc"))
	if exact.wasTruncated() || exact.String() != "abc" {
		t.Fatalf("exact = %q truncated = %v", exact.String(), exact.wasTruncated())
	}

	cut := newLimitedBuffer(2)
	_, _ = cut.Write([]byte("é!")) // é is two bytes; the cut keeps it whole
	_, _ = cut.Write([]byte("x"))
	if !strings.HasPrefix(cut.String(), "é\n[output") {
		t.Fatalf("cut = %q", cut.String())
	}
	broken := newLimitedBuffer(1)
	_, _ = broken.Write([]byte("é"))
	if !strings.HasPrefix(broken.String(), "\uFFFD") {
		t.Fatalf("broken = %q", broken.String())
	}
}
