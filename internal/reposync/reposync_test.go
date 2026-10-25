package reposync

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindRootWalksUpToGitDir(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "api")
	nested := filepath.Join(root, "cmd", "sbox")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o750); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	gotRoot, gotName, err := FindRoot(nested)
	if err != nil {
		t.Fatalf("FindRoot: %v", err)
	}
	if gotRoot != root || gotName != "api" {
		t.Fatalf("FindRoot = %q, %q", gotRoot, gotName)
	}
}

func TestFindRootMissingGit(t *testing.T) {
	t.Parallel()

	_, _, err := FindRoot(t.TempDir())
	if !errors.Is(err, ErrNotRepository) {
		t.Fatalf("err = %v", err)
	}
}

func TestFindRootRejectsGitFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: /other\n"), 0o600); err != nil {
		t.Fatalf("write .git: %v", err)
	}

	_, _, err := FindRoot(dir)
	if !errors.Is(err, ErrGitDirFile) {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateRepoName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		repo    string
		wantErr bool
	}{
		{name: "simple", repo: "api"},
		{name: "dotted", repo: "my.repo"},
		{name: "spaces", repo: "my repo"},
		{name: "empty", repo: "", wantErr: true},
		{name: "dot", repo: ".", wantErr: true},
		{name: "dotdot", repo: "..", wantErr: true},
		{name: "slash", repo: "a/b", wantErr: true},
		{name: "backslash", repo: `a\b`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateRepoName(tt.repo)
			if tt.wantErr && !errors.Is(err, ErrInvalidName) {
				t.Fatalf("err = %v", err)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestSafeRelRejectsEscape(t *testing.T) {
	t.Parallel()

	if _, err := safeRel("../secret"); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("parent err = %v", err)
	}
	if _, err := safeRel("/etc/passwd"); !errors.Is(err, ErrPathEscape) {
		t.Fatalf("absolute err = %v", err)
	}
	got, err := safeRel("cmd/main.go")
	if err != nil || got != "cmd/main.go" {
		t.Fatalf("safeRel = %q, %v", got, err)
	}
}

func TestArchiveRespectsGitignoreAndIncludesGit(t *testing.T) {
	root := initRepo(t)
	isolateGitConfig(t)
	writeFile(t, root, "tracked.txt", "tracked\n")
	git(t, root, "add", "tracked.txt")
	writeFile(t, root, ".gitignore", "ignored.txt\nforced.txt\n")
	writeFile(t, root, "ignored.txt", "skip\n")
	writeFile(t, root, "forced.txt", "keep\n")
	writeFile(t, root, "extra.txt", "extra\n")
	git(t, root, "add", "-f", "forced.txt")
	git(t, root, "add", ".gitignore")
	if err := os.Symlink("tracked.txt", filepath.Join(root, "link.txt")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	git(t, root, "add", "link.txt")
	addSubmodule(t, root)

	rc, err := Archive(t.Context(), root, "api")
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	t.Cleanup(func() { _ = rc.Close() })
	assertArchive(t, readTar(t, rc))
}

func addSubmodule(t *testing.T, root string) {
	t.Helper()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o750); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	writeFile(t, root, "sub/secret.txt", "hidden\n")
	git(t, root, "update-index", "--add", "--cacheinfo", "160000,e69de29bb2d1d6434b8b29ae775ad8c2e48c5391,sub")
}

func assertArchive(t *testing.T, entries map[string]tarEntry) {
	t.Helper()
	assertRegular(t, entries, "api/tracked.txt", "tracked\n")
	assertRegular(t, entries, "api/extra.txt", "extra\n")
	assertRegular(t, entries, "api/forced.txt", "keep\n")
	assertRegular(t, entries, "api/.git/HEAD", "")
	if _, ok := entries["api/ignored.txt"]; ok {
		t.Fatal("gitignored file was archived")
	}
	if _, ok := entries["api/sub/secret.txt"]; ok {
		t.Fatal("submodule file was archived")
	}
	link, ok := entries["api/link.txt"]
	if !ok {
		t.Fatal("missing symlink")
	}
	if link.Typeflag != tar.TypeSymlink || link.Linkname != "tracked.txt" {
		t.Fatalf("symlink = %q type %q", link.Linkname, link.Typeflag)
	}
	for name := range entries {
		if strings.Contains(name, "..") || strings.HasPrefix(name, "/") || (!strings.HasPrefix(name, "api/") && name != "api") {
			t.Fatalf("unsafe tar name %q", name)
		}
	}
}

func isolateGitConfig(t *testing.T) {
	t.Helper()
	global := filepath.Join(t.TempDir(), "empty-gitconfig")
	if err := os.WriteFile(global, nil, 0o600); err != nil {
		t.Fatalf("gitconfig: %v", err)
	}
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_SYSTEM", global)
}

func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init")
	return root
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	//gosec:disable G204 -- test runs git with discrete argv, not a shell
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+filepath.Join(dir, ".gitconfig-none"),
		"GIT_AUTHOR_NAME=sbox",
		"GIT_AUTHOR_EMAIL=sbox@example.com",
		"GIT_COMMITTER_NAME=sbox",
		"GIT_COMMITTER_EMAIL=sbox@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func writeFile(t *testing.T, root, name, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

const maxTarEntry = 1 << 20

type tarEntry struct {
	Typeflag byte
	Linkname string
	Body     string
}

func readTar(t *testing.T, r io.Reader) map[string]tarEntry {
	t.Helper()
	gz, err := gzip.NewReader(r)
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	t.Cleanup(func() { _ = gz.Close() })
	tr := tar.NewReader(gz)
	entries := make(map[string]tarEntry)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("tar: %v", err)
		}
		var buf bytes.Buffer
		n, err := io.Copy(&buf, io.LimitReader(tr, maxTarEntry+1))
		if err != nil {
			t.Fatalf("read %s: %v", hdr.Name, err)
		}
		if n > maxTarEntry {
			t.Fatalf("%s exceeds test size limit", hdr.Name)
		}
		entries[hdr.Name] = tarEntry{Typeflag: hdr.Typeflag, Linkname: hdr.Linkname, Body: buf.String()}
	}
	return entries
}

func assertRegular(t *testing.T, entries map[string]tarEntry, name, body string) {
	t.Helper()
	ent, ok := entries[name]
	if !ok {
		t.Fatalf("missing %s", name)
	}
	if ent.Typeflag != tar.TypeReg {
		t.Fatalf("%s type = %q", name, ent.Typeflag)
	}
	if body != "" && ent.Body != body {
		t.Fatalf("%s body = %q", name, ent.Body)
	}
}
