package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/perylemke/kaxa/internal/registry"
)

// pickerHome builds a home with a Git repo, a plain folder, a hidden folder, and a file.
func pickerHome(t *testing.T) (home, repo string) {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo = filepath.Join(home, "app")
	for _, dir := range []string{filepath.Join(repo, ".git"), filepath.Join(home, "docs"), filepath.Join(home, ".cache")} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, "note.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return home, repo
}

func getDirs(t *testing.T, h http.Handler, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	target := "/api/dirs"
	if path != "-" {
		target += "?path=" + url.QueryEscape(path)
	}
	return do(t, h, s, http.MethodGet, target, nil)
}

func TestDirsListsVisibleFolders(t *testing.T) {
	t.Parallel()

	home, repo := pickerHome(t)
	s, h := newServer(t, nil, Options{Home: home, WorkDir: repo})

	rec := getDirs(t, h, s, "-")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	var got dirListing
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Path != repo || !got.Repo || got.Parent != home || got.Home != home {
		t.Fatalf("listing = %+v", got)
	}

	requireHomeListing(t, getDirs(t, h, s, home))
}

func requireHomeListing(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	var got dirListing
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Parent != "" || got.Repo || len(got.Entries) != 2 {
		t.Fatalf("home = %+v", got)
	}
	if got.Entries[0].Name != "app" || !got.Entries[0].Repo || got.Entries[1].Name != "docs" || got.Entries[1].Repo {
		t.Fatalf("entries = %+v", got.Entries)
	}
}

func TestDirsStartsAtHomeWhenWorkDirIsOutside(t *testing.T) {
	t.Parallel()

	home, _ := pickerHome(t)
	outside, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s, h := newServer(t, nil, Options{Home: home, WorkDir: outside})
	var got dirListing
	if err := json.Unmarshal(getDirs(t, h, s, "-").Body.Bytes(), &got); err != nil || got.Path != home {
		t.Fatalf("listing = %+v err %v", got, err)
	}
}

func TestDirsRefusals(t *testing.T) {
	t.Parallel()

	home, _ := pickerHome(t)
	outside := t.TempDir()
	if runtime.GOOS != "windows" {
		if err := os.Symlink(outside, filepath.Join(home, "escape")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(home, "missing"), filepath.Join(home, "dangling")); err != nil {
			t.Fatal(err)
		}
	}
	s, h := newServer(t, nil, Options{Home: home})
	cases := []struct {
		name string
		path string
		want int
	}{
		{name: "parent of home", path: filepath.Dir(home), want: http.StatusForbidden},
		{name: "parent segments", path: filepath.Join(home, "docs", "..", ".."), want: http.StatusForbidden},
		{name: "unrelated", path: outside, want: http.StatusForbidden},
		{name: "no such folder", path: filepath.Join(home, "nope"), want: http.StatusNotFound},
		{name: "file", path: filepath.Join(home, "note.txt"), want: http.StatusBadRequest},
	}
	if runtime.GOOS != "windows" {
		cases = append(cases, struct {
			name string
			path string
			want int
		}{name: "symlink out", path: filepath.Join(home, "escape"), want: http.StatusForbidden})
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if rec := getDirs(t, h, s, tt.path); rec.Code != tt.want {
				t.Fatalf("status = %d %s", rec.Code, rec.Body)
			}
		})
	}

	var got dirListing
	if err := json.Unmarshal(getDirs(t, h, s, home).Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, entry := range got.Entries {
		if entry.Name == "escape" || entry.Name == "dangling" || entry.Name == "note.txt" || entry.Name == ".cache" {
			t.Fatalf("entry %q must not be listed", entry.Name)
		}
	}
}

func TestDirsRequiresSession(t *testing.T) {
	t.Parallel()

	home, _ := pickerHome(t)
	s, h := newServer(t, nil, Options{Home: home})
	req := httptest.NewRequest(http.MethodGet, "/api/dirs", nil)
	req.Host = s.host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestDirsTruncatesLongListings(t *testing.T) {
	t.Parallel()

	home, _ := pickerHome(t)
	for i := range maxDirEntries + 1 {
		if err := os.Mkdir(filepath.Join(home, fmt.Sprintf("z%04d", i)), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	s, h := newServer(t, nil, Options{Home: home})
	var got dirListing
	if err := json.Unmarshal(getDirs(t, h, s, home).Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Truncated || len(got.Entries) != maxDirEntries {
		t.Fatalf("truncated = %v entries = %d", got.Truncated, len(got.Entries))
	}
}

func TestDirsUnreadableFolder(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permissions are not enforced here")
	}
	home, _ := pickerHome(t)
	locked := filepath.Join(home, "locked")
	if err := os.Mkdir(locked, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	//gosec:disable G302 -- the test restores the folder so TempDir can remove it
	t.Cleanup(func() { _ = os.Chmod(locked, 0o750) })
	s, h := newServer(t, nil, Options{Home: home})
	if rec := getDirs(t, h, s, locked); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
}

func TestDirsWithoutHome(t *testing.T) {
	t.Parallel()

	s, h := newServer(t, nil, Options{})
	s.home = ""
	if rec := getDirs(t, h, s, "-"); rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	if err := s.checkDirs("/tmp"); err == nil {
		t.Fatal("a path must be refused without a home")
	}
}

func TestCreateRefusesDirOutsideHome(t *testing.T) {
	t.Parallel()

	home, _ := pickerHome(t)
	outside := t.TempDir()
	fp := &fakeProvider{name: testProvider}
	s, h := newServer(t, capable{fp}, Options{Home: home})
	for name, dir := range map[string]string{"outside": outside, "second outside": home + "," + outside} {
		t.Run(name, func(t *testing.T) {
			rec := do(t, h, s, http.MethodPost, "/api/sandboxes", createBody{Name: testSandbox, Provider: testProvider, Sync: true, Dir: dir})
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestNewDefaultsHomeAndWorkDir(t *testing.T) {
	t.Parallel()

	s, err := New(Options{Registry: registry.New()})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := os.Getwd()
	if s.workDir != want {
		t.Fatalf("workDir = %q, want %q", s.workDir, want)
	}
	if home, _ := os.UserHomeDir(); home != "" && s.home == "" {
		t.Fatal("home must default to the user's home directory")
	}
	if got := resolveHome(filepath.Join(t.TempDir(), "gone")); got == "" {
		t.Fatal("an unresolvable home keeps its raw path")
	}
}
