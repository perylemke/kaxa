package blaxel

import (
	"slices"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestComputeOptions(t *testing.T) {
	t.Parallel()

	got, ok := sandbox.ComputeOptionsOf(New())
	if !ok || !slices.Equal(got.MemoryMiB, []int{1024, 2048, 4096, 8192}) || len(got.CPUs) != 0 || len(got.Sizes) != 0 {
		t.Fatalf("options = %+v ok = %v", got, ok)
	}
}
