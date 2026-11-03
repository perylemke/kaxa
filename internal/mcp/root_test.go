package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/usererr"
)

func TestRootFromEnvPrefersVariable(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	wd := filepath.Join(home, "wd")
	project := filepath.Join(home, "project")
	if err := os.Mkdir(project, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := RootFromEnv(project, wd, home)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("root = %q, want %q", got, want)
	}
}

func TestRootFromEnvEmptyUsesWorkingDirectory(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	got, err := RootFromEnv("  ", home, home)
	if err != nil {
		t.Fatal(err)
	}
	if got != home {
		t.Fatalf("root = %q, want the working directory", got)
	}
}

func TestRootFromEnvResolvesRelativePath(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	wd := filepath.Join(home, "wd")
	project := filepath.Join(wd, "project")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := RootFromEnv("project", wd, home)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("root = %q, want %q", got, want)
	}
}

func TestRootFromEnvRejectsBroadOrMissing(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	wd := filepath.Join(home, "wd")
	if err := os.Mkdir(wd, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(wd, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.VolumeName(home) + string(filepath.Separator)

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "home", raw: home, want: "too broad"},
		{name: "filesystem root", raw: root, want: "too broad"},
		{name: "missing", raw: filepath.Join(wd, "missing"), want: "was not found"},
		{name: "file", raw: file, want: "not a directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := RootFromEnv(tt.raw, wd, home)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestClientRootRefusesHome(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	_, err := ClientRoot("", home, home)
	ue, ok := usererr.As(err)
	if !ok || !strings.Contains(ue.Message, "too broad") || !strings.Contains(ue.Hint, "--root") {
		t.Fatalf("err = %v", err)
	}
}
