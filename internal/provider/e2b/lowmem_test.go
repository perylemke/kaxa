package e2b

import (
	"slices"
	"testing"

	"github.com/perylemke/kaxa/internal/project"
)

const cargo = "cargo"

func TestDownloadOnly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		step project.Step
		want [][]string
	}{
		{"go", project.Step{Kind: project.KindGo, Commands: [][]string{{"go", "build", "./..."}}}, [][]string{{"go", "mod", "download"}}},
		{"rust", project.Step{Kind: project.KindRust, Commands: [][]string{{cargo, "build"}}}, [][]string{{cargo, "fetch"}}},
		{"rust locked", project.Step{Kind: project.KindRust, Commands: [][]string{{cargo, "build", "--locked"}}}, [][]string{{cargo, "fetch", "--locked"}}},
		{"node", project.Step{Kind: project.KindNode, Commands: [][]string{{"npm", "ci"}}}, [][]string{{"npm", "ci"}}},
		{"python", project.Step{Kind: project.KindPython, Commands: [][]string{{"pip", "install"}}}, [][]string{{"pip", "install"}}},
		{"short argv", project.Step{Kind: project.KindGo, Commands: [][]string{{"go"}}}, [][]string{{"go"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			orig := slices.Clone(tt.step.Commands[0])
			got := downloadOnly([]project.Step{tt.step})
			if len(got) != 1 || got[0].Kind != tt.step.Kind || !slices.EqualFunc(got[0].Commands, tt.want, slices.Equal) {
				t.Fatalf("got %+v, want %v", got, tt.want)
			}
			if !slices.Equal(tt.step.Commands[0], orig) {
				t.Fatalf("input modified: %v", tt.step.Commands[0])
			}
		})
	}
}
