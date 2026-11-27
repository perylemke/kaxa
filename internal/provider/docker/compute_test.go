package docker

import (
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestComputeOptionsAreAcceptedPairs(t *testing.T) {
	t.Parallel()

	got, ok := sandbox.ComputeOptionsOf(New())
	if !ok || len(got.Sizes) != 5 || len(got.CPUs) != 0 || got.CustomCPU || got.CustomMemory {
		t.Fatalf("options = %+v ok = %v", got, ok)
	}
	for _, size := range got.Sizes {
		cpus, mem, err := dockerResources(sandbox.Spec{CPU: size.CPU, MemoryMiB: size.MemoryMiB})
		if err != nil || float64(cpus) != size.CPU || mem != size.MemoryMiB {
			t.Fatalf("%s: cpus %d mem %d err %v", size.Key, cpus, mem, err)
		}
	}
}
