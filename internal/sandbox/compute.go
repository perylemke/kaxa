package sandbox

// ComputeSize is a fixed CPU and memory pair a provider offers as one choice.
type ComputeSize struct {
	Key       string  `json:"key"`
	Label     string  `json:"label"`
	CPU       float64 `json:"cpu"`
	MemoryMiB int     `json:"memory_mib"`
}

// ComputeOptions lists the compute choices a provider offers a form. The create
// wizard and kaxa ui read the same lists, so they cannot drift apart.
type ComputeOptions struct {
	// CPUs are the cores offered in a CPU select.
	CPUs []float64 `json:"cpus,omitempty"`
	// MemoryMiB are the memory sizes offered in a memory select.
	MemoryMiB []int `json:"memory_mib,omitempty"`
	// Sizes are fixed pairs, for a provider that accepts only those pairs.
	Sizes []ComputeSize `json:"sizes,omitempty"`
	// CustomCPU allows a CPU value outside CPUs.
	CustomCPU bool `json:"custom_cpu,omitempty"`
	// CustomMemory allows a memory value outside MemoryMiB.
	CustomMemory bool `json:"custom_memory,omitempty"`
}

// ComputePresets marks a provider that publishes its compute choices.
type ComputePresets interface {
	ComputeOptions() ComputeOptions
}

// ComputeOptionsOf returns the compute choices of p and whether it has any.
func ComputeOptionsOf(p Provider) (ComputeOptions, bool) {
	cp, ok := p.(ComputePresets)
	if !ok {
		return ComputeOptions{}, false
	}
	return cp.ComputeOptions(), true
}

// The lists below are the choices of the providers that adjust compute. Each
// provider returns its own from ComputeOptions.
var (
	// DockerSizes are the CPU and memory pairs the Docker Cloud Sandboxes API accepts.
	DockerSizes = []ComputeSize{
		{Key: "micro", Label: "micro (1 CPU, 2048 MiB)", CPU: 1, MemoryMiB: 2048},
		{Key: "small", Label: "small (2 CPUs, 4096 MiB)", CPU: 2, MemoryMiB: 4096},
		{Key: "medium", Label: "medium (4 CPUs, 8192 MiB)", CPU: 4, MemoryMiB: 8192},
		{Key: "large", Label: "large (8 CPUs, 16384 MiB)", CPU: 8, MemoryMiB: 16384},
		{Key: "xl", Label: "xl (16 CPUs, 32768 MiB)", CPU: 16, MemoryMiB: 32768},
	}
	// ModalCPUs are the cores offered for Modal.
	ModalCPUs = []float64{0.5, 1, 2, 4}
	// ModalMemoryMiB are the memory sizes offered for Modal.
	ModalMemoryMiB = []int{1024, 2048, 4096, 8192}
	// BlaxelMemoryMiB are the memory sizes offered for Blaxel.
	BlaxelMemoryMiB = []int{1024, 2048, 4096, 8192}
)
