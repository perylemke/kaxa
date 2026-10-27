package project_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/perylemke/sbox/internal/project"
)

const (
	detectBuild        = "build"
	detectInstall      = "install"
	detectRequirements = "requirements.txt"
	detectGoMod        = "go.mod"
	detectCargoToml    = "Cargo.toml"
	detectPackageJSON  = "package.json"
)

func TestDetectNone(t *testing.T) {
	t.Parallel()

	got, err := project.Detect(t.TempDir())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("steps = %#v", got)
	}
}

func TestDetectOrderAndCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files []string
		want  []project.Step
	}{
		{
			name:  "go",
			files: []string{detectGoMod},
			want: []project.Step{{
				Kind:     project.KindGo,
				Commands: [][]string{{"go", detectBuild, "./..."}},
			}},
		},
		{
			name:  "rust locked",
			files: []string{detectCargoToml, "Cargo.lock"},
			want: []project.Step{{
				Kind:     project.KindRust,
				Commands: [][]string{{"cargo", detectBuild, "--locked"}},
			}},
		},
		{
			name:  "rust unlocked",
			files: []string{detectCargoToml},
			want: []project.Step{{
				Kind:     project.KindRust,
				Commands: [][]string{{"cargo", detectBuild}},
			}},
		},
		{
			name:  "python pyproject",
			files: []string{"pyproject.toml", detectRequirements},
			want: []project.Step{{
				Kind:     project.KindPython,
				Commands: [][]string{{"uv", "sync"}},
			}},
		},
		{
			name:  "python uv lock",
			files: []string{"uv.lock"},
			want: []project.Step{{
				Kind:     project.KindPython,
				Commands: [][]string{{"uv", "sync"}},
			}},
		},
		{
			name:  "python requirements",
			files: []string{detectRequirements},
			want: []project.Step{{
				Kind: project.KindPython,
				Commands: [][]string{
					{"uv", "venv", ".venv"},
					{"uv", "pip", detectInstall, "--python", ".venv/bin/python", "-r", detectRequirements},
				},
			}},
		},
		{
			name:  "node lock",
			files: []string{detectPackageJSON, "package-lock.json"},
			want: []project.Step{{
				Kind:     project.KindNode,
				Commands: [][]string{{"npm", "ci"}},
			}},
		},
		{
			name:  "node",
			files: []string{detectPackageJSON},
			want: []project.Step{{
				Kind:     project.KindNode,
				Commands: [][]string{{"npm", detectInstall}},
			}},
		},
		{
			name:  "all",
			files: []string{detectPackageJSON, detectRequirements, detectCargoToml, detectGoMod},
			want: []project.Step{
				{Kind: project.KindGo, Commands: [][]string{{"go", detectBuild, "./..."}}},
				{Kind: project.KindRust, Commands: [][]string{{"cargo", detectBuild}}},
				{Kind: project.KindPython, Commands: [][]string{
					{"uv", "venv", ".venv"},
					{"uv", "pip", detectInstall, "--python", ".venv/bin/python", "-r", detectRequirements},
				}},
				{Kind: project.KindNode, Commands: [][]string{{"npm", detectInstall}}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			for _, name := range tt.files {
				if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o600); err != nil {
					t.Fatalf("write %s: %v", name, err)
				}
			}
			got, err := project.Detect(root)
			if err != nil {
				t.Fatalf("Detect: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("steps = %#v", got)
			}
		})
	}
}

func TestDetectIgnoresNestedManifest(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	nested := filepath.Join(root, "services")
	if err := os.Mkdir(nested, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, detectGoMod), []byte("module x\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := project.Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("steps = %#v", got)
	}
}
