package lifecycle_test

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

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/sandbox"
)

const (
	boxName = "box"
	uvicorn = "uvicorn"
	bigTpl  = "big"
	e2bName = "e2b"

	stepCreate  = "create"
	stepSetup   = "setup"
	stepPublish = "publish"
	stepPublic  = "public"
)

type recorder struct {
	created    sandbox.Spec
	destroyed  []sandbox.ID
	synced     bool
	setups     int
	published  bool
	workDir    string
	port       int
	env        []string
	publicSet  bool
	steps      []string
	setupSteps []project.Step
}

type basicProvider struct{ rec *recorder }

const errReserved = "reserved"

func (b basicProvider) Name() string { return "fake" }
func (b basicProvider) Create(_ context.Context, spec sandbox.Spec) (sandbox.ID, error) {
	b.rec.created = spec
	b.rec.steps = append(b.rec.steps, stepCreate)
	return sandbox.ID(spec.Name), nil
}
func (basicProvider) List(context.Context) ([]sandbox.Info, error) { return nil, nil }
func (basicProvider) Inspect(context.Context, sandbox.ID) (sandbox.Detail, error) {
	return sandbox.Detail{}, nil
}

func (basicProvider) Exec(context.Context, sandbox.ID, sandbox.ExecOptions) error {
	return nil
}

func (b basicProvider) Destroy(_ context.Context, id sandbox.ID) error {
	b.rec.destroyed = append(b.rec.destroyed, id)
	return nil
}

type fullProvider struct {
	basicProvider
	syncErr    error
	setupErr   error
	publishErr error
	publicErr  error
}

func (f *fullProvider) PrepareWorkspace(context.Context, sandbox.ID) error { return nil }

func (f *fullProvider) SyncWorkspace(_ context.Context, _ sandbox.ID, archive io.Reader) error {
	if archive != nil {
		if _, err := io.Copy(io.Discard, archive); err != nil {
			return err
		}
	}
	f.rec.synced = true
	f.rec.steps = append(f.rec.steps, stepSync)
	return f.syncErr
}

func (f *fullProvider) SetupProjects(_ context.Context, _ sandbox.ID, _ string, steps []project.Step) error {
	f.rec.setups++
	f.rec.steps = append(f.rec.steps, stepSetup)
	f.rec.setupSteps = append([]project.Step(nil), steps...)
	return f.setupErr
}

func (f *fullProvider) PublishAPI(_ context.Context, _ sandbox.ID, workDir string, port int, _, env []string) error {
	f.rec.published = true
	f.rec.workDir = workDir
	f.rec.port = port
	f.rec.env = append([]string(nil), env...)
	f.rec.steps = append(f.rec.steps, stepPublish)
	return f.publishErr
}

func (f *fullProvider) SetURLAuth(_ context.Context, _ sandbox.ID, auth string) error {
	f.rec.publicSet = auth == sandbox.URLAuthPublic
	f.rec.steps = append(f.rec.steps, stepPublic)
	return f.publicErr
}

func (*fullProvider) SupportsCPU()    {}
func (*fullProvider) SupportsMemory() {}

func (f *fullProvider) HomeDir() string { return "/home/box" }
func (f *fullProvider) RepoDir(repo string) (string, error) {
	return "/home/box/ws/" + repo, nil
}

func newFull() (*fullProvider, *recorder) {
	rec := &recorder{}
	return &fullProvider{rec: rec}, rec
}

func gitRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	root, name, err := lifecycle.ResolveRepo(dir, true)
	if err != nil {
		t.Fatalf("resolve repo: %v", err)
	}
	return root, name
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(dir+"/"+name, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestCreateLabelsAndOrder(t *testing.T) {
	t.Parallel()

	p, rec := newFull()
	root, name := gitRepo(t)
	writeFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.27\n")
	writeFile(t, root, "pyproject.toml", "[project]\nname = \"demo\"\ndependencies = [\"fastapi>=0.115\"]\n")

	res, err := lifecycle.Create(context.Background(), p, lifecycle.Request{
		Name: boxName, Sync: true, API: true, APIPort: 8000, APICmd: []string{uvicorn}, Public: true,
		Root: root, Repo: name,
	}, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	want := []string{stepCreate, stepSync, stepSetup, stepPublish, stepPublic}
	if !slices.Equal(rec.steps, want) {
		t.Fatalf("steps = %v, want %v", rec.steps, want)
	}
	if !slices.Contains(rec.created.Labels, sandbox.SyncLabel) || !slices.Contains(rec.created.Labels, labelKaxa) {
		t.Fatalf("labels = %v", rec.created.Labels)
	}
	if res.ID != boxName || res.WorkDir != "/home/box/ws/"+name {
		t.Fatalf("result = %+v", res)
	}
	if rec.workDir != res.WorkDir || rec.port != 8000 || !rec.publicSet {
		t.Fatalf("publish dir=%q port=%d public=%v", rec.workDir, rec.port, rec.publicSet)
	}
}

func TestCreateSyncMultipleRepos(t *testing.T) {
	t.Parallel()

	p, rec := newFull()
	base := t.TempDir()
	r1 := filepath.Join(base, "a")
	r2 := filepath.Join(base, "b")
	for _, d := range []string{r1, r2} {
		cmd := exec.Command("git", "init", "-q")
		cmd.Dir = d
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git init: %v\n%s", err, out)
		}
		writeFile(t, d, "go.mod", "module x\n\ngo 1.27\n")
	}
	root1, name1, err := lifecycle.ResolveRepo(r1, true)
	if err != nil {
		t.Fatal(err)
	}
	root2, name2, err := lifecycle.ResolveRepo(r2, true)
	if err != nil {
		t.Fatal(err)
	}
	if name1 == name2 {
		t.Fatalf("need distinct repo names, got %q twice", name1)
	}

	res, err := lifecycle.Create(context.Background(), p, lifecycle.Request{
		Name: boxName, Sync: true,
		Repos: []lifecycle.RepoCopy{{Root: root1, Repo: name1}, {Root: root2, Repo: name2}},
	}, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var syncCount int
	for _, s := range rec.steps {
		if s == stepSync {
			syncCount++
		}
	}
	if syncCount != 2 {
		t.Fatalf("want 2 sync steps, got %v", rec.steps)
	}
	if rec.setups != 2 {
		t.Fatalf("want 2 setups, got setups=%d steps=%v", rec.setups, rec.steps)
	}
	if res.WorkDir != "/home/box/ws/"+name1 {
		t.Fatalf("WorkDir = %q", res.WorkDir)
	}
}

func TestCreateWithoutSyncPublishesFromHome(t *testing.T) {
	t.Parallel()

	p, rec := newFull()
	res, err := lifecycle.Create(context.Background(), p, lifecycle.Request{
		Name: boxName, API: true, APIPort: 8000, APICmd: []string{uvicorn},
	}, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if rec.workDir != "/home/box" || res.WorkDir != "" {
		t.Fatalf("publish dir = %q result dir = %q", rec.workDir, res.WorkDir)
	}
}

func TestCreateSyncFailureDestroysSandbox(t *testing.T) {
	t.Parallel()

	p, rec := newFull()
	p.syncErr = errors.New("copy failed")
	root, name := gitRepo(t)

	res, err := lifecycle.Create(context.Background(), p, lifecycle.Request{
		Name: boxName, Sync: true, Root: root, Repo: name,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "copy failed") {
		t.Fatalf("err = %v", err)
	}
	if !slices.Equal(rec.destroyed, []sandbox.ID{boxName}) {
		t.Fatalf("destroyed = %v", rec.destroyed)
	}
	if res.ID != "" {
		t.Fatalf("result kept id %q after destroy", res.ID)
	}
}

func TestCreateSetupFailureDestroysSandbox(t *testing.T) {
	t.Parallel()

	p, rec := newFull()
	p.setupErr = errors.New("go build failed")
	root, name := gitRepo(t)
	writeFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.27\n")

	_, err := lifecycle.Create(context.Background(), p, lifecycle.Request{
		Name: boxName, Sync: true, Root: root, Repo: name,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "go build failed") {
		t.Fatalf("err = %v", err)
	}
	if !slices.Equal(rec.destroyed, []sandbox.ID{boxName}) {
		t.Fatalf("destroyed = %v", rec.destroyed)
	}
}

func TestCreatePublishFailureDestroysSandbox(t *testing.T) {
	t.Parallel()

	p, rec := newFull()
	p.publishErr = errors.New("service failed")

	_, err := lifecycle.Create(context.Background(), p, lifecycle.Request{
		Name: boxName, API: true, APIPort: 8000, APICmd: []string{uvicorn},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "service failed") {
		t.Fatalf("err = %v", err)
	}
	if !slices.Equal(rec.destroyed, []sandbox.ID{boxName}) {
		t.Fatalf("destroyed = %v", rec.destroyed)
	}
}

func TestCreatePublicFailureKeepsSandbox(t *testing.T) {
	t.Parallel()

	p, rec := newFull()
	p.publicErr = errors.New("auth failed")

	res, err := lifecycle.Create(context.Background(), p, lifecycle.Request{
		Name: boxName, API: true, APIPort: 8000, APICmd: []string{uvicorn}, Public: true,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "auth failed") {
		t.Fatalf("err = %v", err)
	}
	if len(rec.destroyed) != 0 {
		t.Fatalf("destroyed = %v", rec.destroyed)
	}
	if res.ID != boxName {
		t.Fatalf("result id = %q, want the kept sandbox", res.ID)
	}
}

func TestCreateRunnerLabelsEachSlowStep(t *testing.T) {
	t.Parallel()

	p, _ := newFull()
	root, name := gitRepo(t)
	writeFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.27\n")

	var labels []string
	run := func(ctx context.Context, label string, task func(context.Context) error) error {
		labels = append(labels, label)
		return task(ctx)
	}
	_, err := lifecycle.Create(context.Background(), p, lifecycle.Request{
		Name: boxName, Sync: true, Root: root, Repo: name,
	}, run)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	want := []string{"creating box on fake…", "syncing " + name + "…", "installing " + name + "…"}
	if !slices.Equal(labels, want) {
		t.Fatalf("labels = %v, want %v", labels, want)
	}
}

func TestCheckRejectsMissingCapabilities(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  lifecycle.Request
		want string
	}{
		{name: "sync", req: lifecycle.Request{Name: boxName, Sync: true}, want: "sync"},
		{name: testAPI, req: lifecycle.Request{Name: boxName, API: true}, want: "api publish"},
		{name: "public", req: lifecycle.Request{Name: boxName, Public: true}, want: "public urls"},
		{name: "running", req: lifecycle.Request{Name: boxName, Running: true}, want: "running"},
		{name: "template", req: lifecycle.Request{Name: boxName, Template: bigTpl}, want: "template"},
		{name: "cpu", req: lifecycle.Request{Name: boxName, CPU: 1}, want: "cpu"},
		{name: "memory", req: lifecycle.Request{Name: boxName, MemoryMiB: 1024}, want: "memory"},
		{name: "network block", req: lifecycle.Request{Name: boxName, NetworkBlockAll: true}, want: "network block"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := &recorder{}
			p := basicProvider{rec: rec}
			_, err := lifecycle.Create(context.Background(), p, tt.req, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want+" is not available for fake") {
				t.Fatalf("err = %v", err)
			}
			if rec.created.Name != "" {
				t.Fatalf("created %q before rejecting", rec.created.Name)
			}
		})
	}
}

func TestCheckRejectsCLIProjectForAPI(t *testing.T) {
	t.Parallel()

	p, rec := newFull()
	root, name := gitRepo(t)
	writeFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.27\n")

	_, err := lifecycle.Create(context.Background(), p, lifecycle.Request{
		Name: boxName, Sync: true, API: true, APIPort: 8000, APICmd: []string{uvicorn},
		Root: root, Repo: name,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "not an HTTP API") {
		t.Fatalf("err = %v", err)
	}
	if rec.created.Name != "" || rec.published {
		t.Fatalf("created %q published %v", rec.created.Name, rec.published)
	}
}

func TestCreatePassesAPIPortToSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		req      lifecycle.Request
		wantAPI  bool
		wantPort int
	}{
		{name: "api", req: lifecycle.Request{Name: boxName, API: true, APIPort: 9000, APICmd: []string{uvicorn}}, wantAPI: true, wantPort: 9000},
		{name: "no api ignores port", req: lifecycle.Request{Name: boxName, APIPort: 9000}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, rec := newFull()
			if _, err := lifecycle.Create(context.Background(), p, tt.req, nil); err != nil {
				t.Fatalf("create: %v", err)
			}
			if rec.created.ExposeAPI != tt.wantAPI || rec.created.APIPort != tt.wantPort {
				t.Fatalf("spec = %+v", rec.created)
			}
		})
	}
}

type templateProvider struct{ *fullProvider }

func (templateProvider) SupportsTemplate() {}

func TestCreatePassesTemplateToSpec(t *testing.T) {
	t.Parallel()

	full, rec := newFull()
	p := templateProvider{full}
	if _, err := lifecycle.Create(context.Background(), p, lifecycle.Request{Name: boxName, Template: bigTpl}, nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	if rec.created.Template != bigTpl {
		t.Fatalf("spec = %+v, want template big", rec.created)
	}
}

func TestCreatePassesCPUAndMemoryToSpec(t *testing.T) {
	t.Parallel()

	p, rec := newFull()
	if _, err := lifecycle.Create(context.Background(), p, lifecycle.Request{Name: boxName, CPU: 2, MemoryMiB: 4096}, nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	if rec.created.CPU != 2 || rec.created.MemoryMiB != 4096 {
		t.Fatalf("spec = %+v", rec.created)
	}
}

func TestCheckRequiresLayoutForAPI(t *testing.T) {
	t.Parallel()

	rec := &recorder{}
	p := apiOnlyProvider{rec: rec}
	_, err := lifecycle.Create(context.Background(), p, lifecycle.Request{
		Name: boxName, API: true, APIPort: 8000, APICmd: []string{uvicorn},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "workspace layout") {
		t.Fatalf("err = %v", err)
	}
	if rec.created.Name != "" {
		t.Fatalf("created %q", rec.created.Name)
	}
}

type apiOnlyProvider struct{ basicProvider }

func (apiOnlyProvider) PublishAPI(context.Context, sandbox.ID, string, int, []string, []string) error {
	return nil
}

func TestResolveRepo(t *testing.T) {
	t.Parallel()

	root, name := gitRepo(t)
	if err := os.Mkdir(root+"/nested", 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	gotRoot, gotName, err := lifecycle.ResolveRepo(root+"/nested", true)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if gotRoot != root || gotName != name {
		t.Fatalf("root=%q name=%q", gotRoot, gotName)
	}
}

func TestResolveRepoRejectsBadDirs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := dir + "/note.txt"
	writeFile(t, dir, "note.txt", "x")
	plain := t.TempDir()
	tests := []struct {
		name string
		dir  string
		want string
	}{
		{name: "missing", dir: dir + "/missing", want: "does not exist"},
		{name: "file", dir: file, want: "not a directory"},
		{name: "empty", dir: " ", want: "directory is required"},
		{name: "not a repo", dir: plain, want: plain + " is not inside a git repository"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := lifecycle.ResolveRepo(tt.dir, true)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestValidateAPI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		running bool
		port    int
		argv    []string
		want    string
	}{
		{name: "ok", port: 8000, argv: []string{uvicorn, "app:app"}},
		{name: "no command", port: 8000, want: "api command is required"},
		{name: "option as binary", port: 8000, argv: []string{"--http-port", "1"}, want: "not an option"},
		{name: "blank argument", port: 8000, argv: []string{uvicorn, " "}, want: "empty argument"},
		{name: "low port", port: 0, argv: []string{uvicorn}, want: "invalid api port"},
		{name: "high port", port: 70000, argv: []string{uvicorn}, want: "invalid api port"},
		{name: "tmux port", running: true, port: 8080, argv: []string{uvicorn}, want: "8080"},
		{name: "8080 without running", port: 8080, argv: []string{uvicorn}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := lifecycle.ValidateAPI(tt.running, tt.port, tt.argv)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

const (
	modeProd   = "MODE=prod"
	commaValue = "a,b"
)

func TestResolveAPIEnv(t *testing.T) {
	t.Parallel()

	local := map[string]string{
		"INHERITED":  "from-shell",
		"WITH_COMMA": commaValue,
	}
	lookup := func(key string) (string, bool) {
		v, ok := local[key]
		return v, ok
	}
	tests := []struct {
		name    string
		entries []string
		want    []string
		errHas  string
	}{
		{name: "none"},
		{name: "literal", entries: []string{modeProd}, want: []string{modeProd}},
		{name: "empty value", entries: []string{"EMPTY="}, want: []string{"EMPTY="}},
		{name: "value keeps equals", entries: []string{"Q=a=b"}, want: []string{"Q=a=b"}},
		{name: "inherited", entries: []string{"INHERITED"}, want: []string{"INHERITED=from-shell"}},
		{name: "unset inherited", entries: []string{"MISSING"}, errHas: "MISSING is not set"},
		{name: "bad key", entries: []string{"1BAD=x"}, errHas: "invalid api env key"},
		{name: "key with dash", entries: []string{"A-B=x"}, errHas: "invalid api env key"},
		{name: "empty key", entries: []string{"=x"}, errHas: "invalid api env key"},
		{name: "comma literal", entries: []string{"X=a,b"}, errHas: "comma"},
		{name: "comma inherited", entries: []string{"WITH_COMMA"}, errHas: "comma"},
		{name: "newline", entries: []string{"X=a\nb"}, errHas: "control character"},
		{name: "duplicate", entries: []string{"X=1", "X=2"}, errHas: "more than once"},
		{name: "reserved path", entries: []string{"PATH=/bin"}, errHas: errReserved},
		{name: "reserved home", entries: []string{"HOME=/"}, errHas: errReserved},
		{name: "kaxa token", entries: []string{"KAXA_SPRITES_TOKEN"}, errHas: errReserved},
		{name: "modal token", entries: []string{"MODAL_TOKEN_SECRET"}, errHas: errReserved},
		{name: "fly token", entries: []string{"FLY_API_TOKEN=x"}, errHas: errReserved},
		{name: "sprites token", entries: []string{"SPRITES_TOKEN"}, errHas: errReserved},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := lifecycle.ResolveAPIEnv(tt.entries, lookup)
			if tt.errHas != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errHas) {
					t.Fatalf("err = %v, want %q", err, tt.errHas)
				}
				if strings.Contains(err.Error(), commaValue) {
					t.Fatalf("error leaks a value: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCreatePassesAPIEnvToPublisher(t *testing.T) {
	t.Parallel()

	p, rec := newFull()
	_, err := lifecycle.Create(context.Background(), p, lifecycle.Request{
		Name: boxName, API: true, APIPort: 8000, APICmd: []string{uvicorn}, APIEnv: []string{modeProd},
	}, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !slices.Equal(rec.env, []string{modeProd}) {
		t.Fatalf("env = %v", rec.env)
	}
}

type runningProvider struct{ *fullProvider }

func (runningProvider) SupportsRunning() {}

func TestCheckRejectsPublicTerminal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		req     lifecycle.Request
		wantErr bool
	}{
		{name: "running public without api", req: lifecycle.Request{Name: boxName, Running: true, Public: true}, wantErr: true},
		{name: "running public with api", req: lifecycle.Request{Name: boxName, Running: true, Public: true, API: true, APIPort: 8000, APICmd: []string{uvicorn}}},
		{name: "running private", req: lifecycle.Request{Name: boxName, Running: true}},
		{name: "public without running", req: lifecycle.Request{Name: boxName, Public: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			full, rec := newFull()
			_, err := lifecycle.Create(context.Background(), runningProvider{full}, tt.req, nil)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "terminal") {
					t.Fatalf("err = %v", err)
				}
				if rec.created.Name != "" {
					t.Fatalf("created %q before rejecting", rec.created.Name)
				}
				return
			}
			if err != nil {
				t.Fatalf("create: %v", err)
			}
		})
	}
}

func TestResolveReposEmptyUsesWd(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	runGitForRepoTest(t, tmp, "init", "-q")
	goModRel := "go.mod"
	if err := os.WriteFile(filepath.Join(tmp, goModRel), []byte("module x\n\ngo 1.27\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitForRepoTest(t, tmp, "add", goModRel)
	runGitForRepoTest(t, tmp, "commit", "-m", "init")

	got, err := lifecycle.ResolveRepos("", false)
	if err != nil {
		t.Fatalf("ResolveRepos: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Repo != filepath.Base(tmp) {
		t.Fatalf("Repo name = %q", got[0].Repo)
	}
}

func TestResolveReposCommaTwoRepos(t *testing.T) {
	tmp := t.TempDir()
	a := filepath.Join(tmp, "alpha")
	b := filepath.Join(tmp, "beta")
	mkdirGitInitForResolveRepos(t, a, "a")
	mkdirGitInitForResolveRepos(t, b, "b")

	got, err := lifecycle.ResolveRepos(a+","+b, true)
	if err != nil {
		t.Fatalf("ResolveRepos: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	names := []string{got[0].Repo, got[1].Repo}
	if names[0] != "alpha" || names[1] != "beta" {
		t.Fatalf("repos = %#v", got)
	}
}

func TestResolveReposRejectEmptySegment(t *testing.T) {
	tmp := t.TempDir()
	mkdirGitInitForResolveRepos(t, filepath.Join(tmp, "a"), "x")

	_, err := lifecycle.ResolveRepos(tmp+"/a,,", true)
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveReposRejectDuplicateRoot(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "once")
	mkdirGitInitForResolveRepos(t, p, "o")

	_, err := lifecycle.ResolveRepos(p+","+p, true)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveReposRejectDuplicateWorkspaceName(t *testing.T) {
	tmp := t.TempDir()
	a := filepath.Join(tmp, "x", "r1")
	b := filepath.Join(tmp, "y", "r1")
	mkdirGitInitForResolveRepos(t, a, "f")
	mkdirGitInitForResolveRepos(t, b, "f")

	_, err := lifecycle.ResolveRepos(a+","+b, true)
	if err == nil {
		t.Fatal("expected error")
	}
}

func mkdirGitInitForResolveRepos(t *testing.T, dir, fname string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	runGitForRepoTest(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, fname), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitForRepoTest(t, dir, "add", fname)
	runGitForRepoTest(t, dir, "commit", "-m", "init")
}

func runGitForRepoTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:gosec // test helper; argv from fixed tests only
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_DATE=1970-01-01T00:00:00Z", "GIT_COMMITTER_DATE=1970-01-01T00:00:00Z",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
