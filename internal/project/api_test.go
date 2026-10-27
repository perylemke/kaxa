package project_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/perylemke/kaxa/internal/project"
)

type apiCase struct {
	name  string
	files map[string]string
	want  bool
}

var apiCases = []apiCase{
	{name: "empty"},
	{
		name: "go listen",
		files: map[string]string{
			"main.go": "package main\nfunc main() { http.ListenAndServe(\":8080\", nil) }\n",
		},
		want: true,
	},
	{
		name: "go listen tls",
		files: map[string]string{
			"main.go": "package main\nfunc main() { http.ListenAndServeTLS(\":443\", \"c\", \"k\", nil) }\n",
		},
		want: true,
	},
	{
		name: "go server struct",
		files: map[string]string{
			"cmd/api/main.go": "package main\nvar s = &http.Server{Addr: \":8080\"}\n",
		},
		want: true,
	},
	{
		name: "go client",
		files: map[string]string{
			detectGoMod: "module example.com/cli\n\nrequire github.com/spf13/cobra v1.8.0\n",
			"main.go":   "package main\nfunc main() { http.NewRequest(\"GET\", \"/\", nil) }\n",
		},
	},
	{
		name: "go test only",
		files: map[string]string{
			"main_test.go": "package main\nfunc TestServe(t *testing.T) { http.ListenAndServe(\":1\", nil) }\n",
		},
	},
	{
		name: "go gin",
		files: map[string]string{
			detectGoMod: "module example.com/api\n\nrequire github.com/gin-gonic/gin v1.10.0\n",
		},
		want: true,
	},
	{
		name: "go listen in vendor",
		files: map[string]string{
			"vendor/lib.go": "package lib\nfunc X() { http.ListenAndServe(\":1\", nil) }\n",
		},
	},
	{
		name: "python fastapi",
		files: map[string]string{
			"pyproject.toml": "dependencies = [\"fastapi>=0.115\"]\n",
		},
		want: true,
	},
	{
		name: "python typer",
		files: map[string]string{
			"pyproject.toml": "dependencies = [\"typer>=0.12\"]\n",
		},
	},
	{
		name: "python flask requirements",
		files: map[string]string{
			detectRequirements: "flask==3.0.0\n",
		},
		want: true,
	},
	{
		name: "python uvicorn lock",
		files: map[string]string{
			"uv.lock": "name = \"uvicorn\"\n",
		},
		want: true,
	},
	{
		name: "node express",
		files: map[string]string{
			detectPackageJSON: `{"dependencies":{"express":"^4.0.0"}}`,
		},
		want: true,
	},
	{
		name: "node nest dev",
		files: map[string]string{
			detectPackageJSON: `{"devDependencies":{"@nestjs/core":"10.0.0"}}`,
		},
		want: true,
	},
	{
		name: "node commander",
		files: map[string]string{
			detectPackageJSON: `{"dependencies":{"commander":"^12.0.0"}}`,
		},
	},
	{
		name: "rust axum",
		files: map[string]string{
			detectCargoToml: "[dependencies]\naxum = \"0.7\"\n",
		},
		want: true,
	},
	{
		name: "go quoted signal",
		files: map[string]string{
			"scan.go": "package scan\nvar signals = []string{\"http.ListenAndServe\", \"http.Server\"}\n",
		},
	},
	{
		name: "rust clap",
		files: map[string]string{
			detectCargoToml: "[dependencies]\nclap = \"4\"\n",
		},
	},
}

func TestIsAPI(t *testing.T) {
	t.Parallel()

	for _, tt := range apiCases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := writeAPIFixture(t, tt.files)
			got, err := project.IsAPI(root)
			if err != nil {
				t.Fatalf("IsAPI: %v", err)
			}
			if got != tt.want {
				t.Fatalf("IsAPI = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestModuleRootIsNotAPI(t *testing.T) {
	t.Parallel()

	root := moduleRoot(t)
	ok, err := project.IsAPI(root)
	if err != nil {
		t.Fatalf("IsAPI: %v", err)
	}
	if ok {
		t.Fatal("repository was classified as an HTTP API")
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("wd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, detectGoMod)); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func TestIsAPIRejectsBrokenPackageJSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, detectPackageJSON), []byte("{"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := project.IsAPI(root); err == nil {
		t.Fatal("expected parse error")
	}
}

func writeAPIFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}
