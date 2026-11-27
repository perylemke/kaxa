package modal_test

import (
	"slices"
	"testing"

	"github.com/perylemke/kaxa/internal/provider/modal"
	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestComputeOptions(t *testing.T) {
	t.Parallel()

	got, ok := sandbox.ComputeOptionsOf(modal.New())
	if !ok {
		t.Fatal("modal must publish compute options")
	}
	if !slices.Equal(got.CPUs, []float64{0.5, 1, 2, 4}) || !slices.Equal(got.MemoryMiB, []int{1024, 2048, 4096, 8192}) || !got.CustomCPU || !got.CustomMemory || len(got.Sizes) != 0 {
		t.Fatalf("options = %+v", got)
	}
	got.CPUs[0] = 99
	if sandbox.ModalCPUs[0] == 99 {
		t.Fatal("the options must be a copy")
	}
}
