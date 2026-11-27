package sandbox_test

import (
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

type presetProvider struct{ bareProvider }

func (presetProvider) ComputeOptions() sandbox.ComputeOptions {
	return sandbox.ComputeOptions{MemoryMiB: []int{512}}
}

func TestComputeOptionsOf(t *testing.T) {
	t.Parallel()

	if _, ok := sandbox.ComputeOptionsOf(bareProvider{}); ok {
		t.Fatal("a provider without presets must report none")
	}
	got, ok := sandbox.ComputeOptionsOf(presetProvider{})
	if !ok || len(got.MemoryMiB) != 1 || got.MemoryMiB[0] != 512 {
		t.Fatalf("options = %+v ok = %v", got, ok)
	}
}
