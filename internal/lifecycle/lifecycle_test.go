package lifecycle_test

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
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
)

type recorder struct {
	created   sandbox.Spec
	destroyed []sandbox.ID
	synced    bool
	setups    int
	published bool
	workDir   string
	port      int
	env       []string
	publicSet bool
	steps     []string
}

type basicProvider struct{ rec *recorder }

const errReserved = "reserved"

func (b basicProvider) Name() string { return "fake" }
func (b basicProvider) Create(_ context.Context, spec sandbox.Spec) (sandbox.ID, error) {
	b.rec.created = spec
	b.rec.steps = append(b.rec.steps, "create")
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

func (f *fullProvider) SyncWorkspace(_ context.Context, _ sandbox.ID, archive io.Reader) error {
	if archive != nil {
		if _, err := io.Copy(io.Discard, archive); err != nil {
			return err
		}
	}
	f.rec.synced = true
	f.rec.steps = append(f.rec.steps, "sync")
	return f.syncErr
}

func (f *fullProvider) SetupProjects(context.Context, sandbox.ID, string, []project.Step) error {
	f.rec.setups++
	f.rec.steps = append(f.rec.steps, "setup")
	return f.setupErr
}

func (f *fullProvider) PublishAPI(_ context.Context, _ sandbox.ID, workDir string, port int, _, env []string) error {
	f.rec.published = true
	f.rec.workDir = workDir
	f.rec.port = port
	f.rec.env = append([]string(nil), env...)
	f.rec.steps = append(f.rec.steps, "publish")
	return f.publishErr
}

func (f *fullProvider) SetURLAuth(_ context.Context, _ sandbox.ID, auth string) error {
	f.rec.publicSet = auth == sandbox.URLAuthPublic
	f.rec.steps = append(f.rec.steps, "public")
	return f.publicErr
}

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
	want := []string{"create", "sync", "setup", "publish", "public"}
	if !slices.Equal(rec.steps, want) {
		t.Fatalf("steps = %v, want %v", rec.steps, want)
	}
	if !slices.Contains(rec.created.Labels, sandbox.SyncLabel) || !slices.Contains(rec.created.Labels, "kaxa") {
		t.Fatalf("labels = %v", rec.created.Labels)
	}
	if res.ID != boxName || res.WorkDir != "/home/box/ws/"+name {
		t.Fatalf("result = %+v", res)
	}
	if rec.workDir != res.WorkDir || rec.port != 8000 || !rec.publicSet {
		t.Fatalf("publish dir=%q port=%d public=%v", rec.workDir, rec.port, rec.publicSet)
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
		{name: "missing", dir: dir + "/missing", want: "was not found"},
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
