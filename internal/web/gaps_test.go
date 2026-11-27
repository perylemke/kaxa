package web

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestCreateSyncAndAPIOptions(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testProvider}
	repo := gitRepo(t)
	s, h := newServer(t, capable{fp}, Options{Home: filepath.Dir(repo)})

	cases := []struct {
		name string
		body createBody
		want int
	}{
		{name: "dir without sync", body: createBody{Name: testSandbox, Provider: testProvider, Dir: repo}, want: http.StatusBadRequest},
		{name: "no install without sync", body: createBody{Name: testSandbox, Provider: testProvider, NoInstall: true}, want: http.StatusBadRequest},
		{name: "missing repo", body: createBody{Name: testSandbox, Provider: testProvider, Sync: true, Dir: "/no/such/kaxa-ui-repo"}, want: http.StatusBadRequest},
		{name: "sync", body: createBody{Name: testSandbox, Provider: testProvider, Sync: true, NoInstall: true, Dir: repo}, want: http.StatusOK},
		{name: "api command without api", body: createBody{Name: testSandbox, Provider: testProvider, APICmd: []string{"server"}}, want: http.StatusBadRequest},
		{name: "api port without api", body: createBody{Name: testSandbox, Provider: testProvider, APIPort: 9}, want: http.StatusBadRequest},
		{name: "api env without api", body: createBody{Name: testSandbox, Provider: testProvider, APIEnv: []string{"MODE=dev"}}, want: http.StatusBadRequest},
		{name: "reserved api env", body: createBody{Name: testSandbox, Provider: testProvider, API: true, APICmd: []string{"server"}, APIEnv: []string{"KAXA_TOKEN=secret"}}, want: http.StatusBadRequest},
		{name: "api", body: createBody{Name: testSandbox, Provider: testProvider, API: true, APICmd: []string{"server"}}, want: http.StatusOK},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, h, s, http.MethodPost, "/api/sandboxes", tt.body)
			if rec.Code != tt.want {
				t.Fatalf("status = %d %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestActionErrors(t *testing.T) {
	t.Parallel()

	plain := &fakeProvider{name: testProvider}
	s, h := newServer(t, plain, Options{})
	if rec := do(t, h, s, http.MethodPost, "/api/sandboxes/lab/box/upgrade", map[string]string{}); rec.Code != http.StatusBadRequest {
		t.Fatalf("upgrade = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, s, http.MethodPost, "/api/sandboxes/lab/box/url-auth", urlAuthBody{Auth: "public"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("url = %d %s", rec.Code, rec.Body)
	}
	if rec := rawDo(t, h, s, http.MethodPost, "/api/sandboxes/lab/box/destroy", "{"); rec.Code != http.StatusBadRequest {
		t.Fatalf("json = %d %s", rec.Code, rec.Body)
	}
	if rec := rawDo(t, h, s, http.MethodPost, "/api/sandboxes/lab/box/destroy", "{}{}"); rec.Code != http.StatusBadRequest {
		t.Fatalf("extra json = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, s, http.MethodGet, "/api/sandboxes/Lab/box", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("path = %d %s", rec.Code, rec.Body)
	}

	full := &fakeProvider{name: testProvider, upgradeErr: stringError("upgrade failed"), authErr: stringError("auth failed"), destroyErr: stringError("destroy failed")}
	cs, ch := newServer(t, capable{full}, Options{})
	if rec := do(t, ch, cs, http.MethodPost, "/api/sandboxes/lab/box/upgrade", map[string]string{}); rec.Code != http.StatusInternalServerError {
		t.Fatalf("upgrade err = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, ch, cs, http.MethodPost, "/api/sandboxes/lab/box/url-auth", urlAuthBody{Auth: sandbox.URLAuthSprite}); rec.Code != http.StatusInternalServerError {
		t.Fatalf("auth err = %d %s", rec.Code, rec.Body)
	}
	full.authErr = nil
	full.inspectErr = stringError("inspect failed after auth")
	if rec := do(t, ch, cs, http.MethodPost, "/api/sandboxes/lab/box/url-auth", urlAuthBody{Auth: sandbox.URLAuthSprite}); rec.Code != http.StatusInternalServerError {
		t.Fatalf("inspect after auth = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, ch, cs, http.MethodPost, "/api/sandboxes/lab/box/destroy", destroyBody{Confirm: testSandbox}); rec.Code != http.StatusInternalServerError {
		t.Fatalf("destroy err = %d %s", rec.Code, rec.Body)
	}
}

func TestProviderAndListEdges(t *testing.T) {
	t.Parallel()

	off := &fakeProvider{name: testProvider, unavailable: true}
	s, h := newServer(t, off, Options{})
	if rec := do(t, h, s, http.MethodPost, "/api/sandboxes", createBody{Name: testSandbox, Provider: testProvider}); rec.Code != http.StatusBadRequest {
		t.Fatalf("unavailable = %d %s", rec.Code, rec.Body)
	}

	missing, mh := newServer(t, &fakeProvider{name: testProvider}, Options{})
	if rec := do(t, mh, missing, http.MethodPost, "/api/sandboxes", createBody{Name: testSandbox, Provider: "missing"}); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown = %d %s", rec.Code, rec.Body)
	}

	sprites := &fakeProvider{name: "sprites"}
	reg := registry.New()
	mustRegister(t, reg, tokened{sprites})
	ns, nh := newServer(t, nil, Options{Registry: reg})
	if rec := do(t, nh, ns, http.MethodGet, "/api/sandboxes/sprites/box", nil); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "not configured") {
		t.Fatalf("unconfigured = %d %s", rec.Code, rec.Body)
	}

	empty, eh := newServer(t, nil, Options{Registry: registry.New()})
	if rec := do(t, eh, empty, http.MethodGet, "/api/sandboxes", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("no providers = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, eh, empty, http.MethodGet, "/api/providers", nil); rec.Code != http.StatusOK {
		t.Fatalf("empty providers = %d %s", rec.Code, rec.Body)
	}

	onlyReg := registry.New()
	mustRegister(t, onlyReg, &fakeProvider{name: testProvider, list: []sandbox.Info{{ID: testSandbox, Status: testStatus}}})
	osrv, oh := newServer(t, nil, Options{Registry: onlyReg, Provider: testProvider})
	if rec := do(t, oh, osrv, http.MethodGet, "/api/sandboxes", nil); rec.Code != http.StatusOK {
		t.Fatalf("only list = %d %s", rec.Code, rec.Body)
	}
	badOnly, bh := newServer(t, nil, Options{Registry: onlyReg, Provider: "missing"})
	if rec := do(t, bh, badOnly, http.MethodGet, "/api/providers", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown only = %d %s", rec.Code, rec.Body)
	}
}

func TestCreateKeepsTheSandboxWhenInspectFails(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{
		name: testProvider, inspectErr: stringError("inspect failed"),
		detail: sandbox.Detail{
			ID: testSandbox, UpdatedAt: time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC), URL: "https://example.test/box",
		},
	}
	s, h := newServer(t, fp, Options{})
	rec := do(t, h, s, http.MethodPost, "/api/sandboxes", createBody{Name: testSandbox, Provider: testProvider})
	if out := createDone(t, rec); rec.Code != http.StatusOK || out.URL != "" || out.ID != testSandbox {
		t.Fatalf("create = %d %+v", rec.Code, out)
	}

	fp.inspectErr = nil
	fp.detail.Provider = ""
	got := do(t, h, s, http.MethodGet, "/api/sandboxes/lab/box", nil)
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"provider":"lab"`) || !strings.Contains(got.Body.String(), "updated_at") {
		t.Fatalf("detail = %d %s", got.Code, got.Body)
	}
}

func TestRedactSkipsAnEmptySecret(t *testing.T) {
	t.Parallel()

	s, _ := newServer(t, &fakeProvider{name: testProvider}, Options{})
	s.secrets = []string{""}
	if got := s.redact("hello"); got != "hello" {
		t.Fatalf("redact = %q", got)
	}
}

func TestListenRejectsAnInvalidPort(t *testing.T) {
	t.Parallel()

	err := ListenAndServe(t.Context(), Options{Registry: registry.New(), Port: 70000})
	if err == nil {
		t.Fatal("invalid port was accepted")
	}
}

type tokened struct{ *fakeProvider }

func (tokened) Configure(string) error { return sandbox.ErrNotConfigured }

func rawDo(t *testing.T, h http.Handler, srv *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Host = srv.host
	req.Header.Set(headerSession, srv.sessionHex)
	req.Header.Set(headerCSRF, srv.csrfHex)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, exec.Command("git", "init", "-b", "main"))
	runGit(t, dir, exec.Command("git", "config", "user.email", "dev@example.com"))
	runGit(t, dir, exec.Command("git", "config", "user.name", "dev"))
	runGit(t, dir, exec.Command("git", "commit", "--allow-empty", "-m", "init"))
	return dir
}

func runGit(t *testing.T, dir string, cmd *exec.Cmd) {
	t.Helper()
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v %s", cmd.Args, err, out)
	}
}
