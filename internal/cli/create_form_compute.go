package cli

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"charm.land/huh/v2"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

// Wizard-only presets for CPU, memory, and Docker size. Flags and MCP stay numeric.

const (
	formProviderModal  = "modal"
	formProviderDocker = "docker"
	formProviderBlaxel = "blaxel"

	formPresetDefault = "default"

	formPresetCPUOther = "cpu_other"
	formPresetMemOther = "mem_other"

	// formPresetDockerCustom is a size outside the documented pairs (e.g. from flags).
	formPresetDockerCustom = "docker_custom"
)

// dockerSizePreset matches Cloud Sandboxes documented pairs (micro … xl).
type dockerSizePreset struct {
	Key  string
	CPUs int
	Mem  int
	// Label is the prompt option text.
	Label string
}

var dockerWizardSizes = []dockerSizePreset{
	{Key: "micro", CPUs: 1, Mem: 2048, Label: "micro (1 CPU, 2048 MiB)"},
	{Key: "small", CPUs: 2, Mem: 4096, Label: "small (2 CPUs, 4096 MiB)"},
	{Key: "medium", CPUs: 4, Mem: 8192, Label: "medium (4 CPUs, 8192 MiB)"},
	{Key: "large", CPUs: 8, Mem: 16384, Label: "large (8 CPUs, 16384 MiB)"},
	{Key: "xl", CPUs: 16, Mem: 32768, Label: "xl (16 CPUs, 32768 MiB)"},
}

var modalWizardCPUs = []struct {
	Key   string
	Value float64
	Label string
}{
	{"0.5", 0.5, "0.5 cores"},
	{"1", 1, "1 core"},
	{"2", 2, "2 cores"},
	{"4", 4, "4 cores"},
}

var modalWizardMemoryMiB = []int{1024, 2048, 4096, 8192}

var blaxelWizardMemoryMiB = []int{1024, 2048, 4096, 8192}

func dockerSizeKeyFor(cpu float64, mem int) string {
	for _, s := range dockerWizardSizes {
		if int(cpu) == s.CPUs && mem == s.Mem {
			return s.Key
		}
	}
	return ""
}

func seedModalCPUPreset(cpu float64) (preset, input string) {
	if cpu == 0 {
		return formPresetDefault, ""
	}
	s := strconv.FormatFloat(cpu, 'f', -1, 64)
	for _, o := range modalWizardCPUs {
		if cpu == o.Value {
			return o.Key, ""
		}
	}
	return formPresetCPUOther, s
}

func seedModalMemoryPreset(mem int) (preset, input string) {
	if mem == 0 {
		return formPresetDefault, ""
	}
	for _, m := range modalWizardMemoryMiB {
		if mem == m {
			return strconv.Itoa(m), ""
		}
	}
	return formPresetMemOther, strconv.Itoa(mem)
}

func seedBlaxelMemoryPreset(mem int) (preset, input string) {
	if mem == 0 {
		return formPresetDefault, ""
	}
	for _, m := range blaxelWizardMemoryMiB {
		if mem == m {
			return strconv.Itoa(m), ""
		}
	}
	// Value outside the preset list (e.g. from --memory): single extra select option.
	return strconv.Itoa(mem), ""
}

func applyModalFlagsToComputePresets(flags createFlags, r *createFormResult) {
	if flags.CPU == 0 && flags.MemoryMiB == 0 {
		return
	}
	r.cpuPreset, r.cpuInput = seedModalCPUPreset(flags.CPU)
	r.memoryPreset, r.memoryInput = seedModalMemoryPreset(flags.MemoryMiB)
}

func applyDockerFlagsToComputePresets(flags createFlags, r *createFormResult) {
	if flags.CPU == 0 && flags.MemoryMiB == 0 {
		return
	}
	cpu := flags.CPU
	mem := flags.MemoryMiB
	if mem == 0 {
		mem = 4096
	}
	if cpu == 0 {
		cpu = 2
	}
	key := dockerSizeKeyFor(cpu, mem)
	if key != "" {
		r.dockerSizePreset = key
		r.cpuInput, r.memoryInput = "", ""
		return
	}
	r.dockerSizePreset = formPresetDockerCustom
	r.cpuInput = strconv.FormatFloat(cpu, 'f', -1, 64)
	r.memoryInput = strconv.Itoa(mem)
}

func applyBlaxelFlagsToComputePresets(flags createFlags, r *createFormResult) {
	if flags.MemoryMiB == 0 {
		return
	}
	r.memoryPreset, r.memoryInput = seedBlaxelMemoryPreset(flags.MemoryMiB)
}

func applyGenericFlagsToComputePresets(flags createFlags, r *createFormResult) {
	if flags.CPU != 0 {
		r.cpuInput = strconv.FormatFloat(flags.CPU, 'f', -1, 64)
	}
	if flags.MemoryMiB != 0 {
		r.memoryInput = strconv.Itoa(flags.MemoryMiB)
	}
}

func applyFlagsToComputePresets(provider string, flags createFlags, r *createFormResult) {
	if flags.CPU == 0 && flags.MemoryMiB == 0 {
		return
	}
	switch provider {
	case formProviderModal:
		applyModalFlagsToComputePresets(flags, r)
	case formProviderDocker:
		applyDockerFlagsToComputePresets(flags, r)
	case formProviderBlaxel:
		applyBlaxelFlagsToComputePresets(flags, r)
	default:
		applyGenericFlagsToComputePresets(flags, r)
	}
}

func syncComputePresetsForDefault(defaultProvider string, flags createFlags, r *createFormResult) {
	if defaultProvider == "" {
		return
	}
	r.cpuPreset, r.memoryPreset, r.dockerSizePreset = "", "", ""
	r.cpuInput, r.memoryInput = "", ""
	applyFlagsToComputePresets(defaultProvider, flags, r)
}

func syncComputePresetsAfterProviderChange(p sandbox.Provider, flags createFlags, r *createFormResult) {
	r.cpuPreset, r.memoryPreset, r.dockerSizePreset = "", "", ""
	r.cpuInput, r.memoryInput = "", ""
	applyFlagsToComputePresets(p.Name(), flags, r)
}

func resolveWizardCompute(provider string, r *createFormResult) error {
	switch provider {
	case formProviderDocker:
		return resolveDockerWizardCompute(r)
	case formProviderModal:
		return resolveModalWizardCompute(r)
	case formProviderBlaxel:
		return resolveBlaxelWizardCompute(r)
	default:
		cpu, mem, err := parsedCreateCompute(r.cpuInput, r.memoryInput)
		if err != nil {
			return err
		}
		r.CPU, r.MemoryMiB = cpu, mem
		return nil
	}
}

func resolveDockerWizardCompute(r *createFormResult) error {
	if r.dockerSizePreset == "" || r.dockerSizePreset == formPresetDefault {
		r.CPU, r.MemoryMiB = 0, 0
		return nil
	}
	if r.dockerSizePreset == formPresetDockerCustom {
		cpu, mem, err := parsedCreateCompute(r.cpuInput, r.memoryInput)
		if err != nil {
			return err
		}
		if cpu == 0 || mem == 0 {
			return usererr.New(
				"Docker size needs both CPU and memory (MiB)",
				"pick a size from the list or enter both values for a custom pair",
			)
		}
		if cpu != float64(int(cpu)) || cpu < 1 {
			return usererr.New(
				"Docker CPU must be a positive whole number of cores",
				"pick a size from the list",
			)
		}
		r.CPU = cpu
		r.MemoryMiB = mem
		return nil
	}
	for _, s := range dockerWizardSizes {
		if s.Key == r.dockerSizePreset {
			r.CPU = float64(s.CPUs)
			r.MemoryMiB = s.Mem
			return nil
		}
	}
	return usererr.New(
		fmt.Sprintf("invalid Docker size preset %q", r.dockerSizePreset),
		"choose a size from the list",
	)
}

func resolveModalWizardCompute(r *createFormResult) error {
	cpu, err := modalCPUFromPreset(r.cpuPreset, r.cpuInput)
	if err != nil {
		return err
	}
	mem, err := modalMemoryFromPreset(r.memoryPreset, r.memoryInput)
	if err != nil {
		return err
	}
	r.CPU, r.MemoryMiB = cpu, mem
	return nil
}

func modalCPUFromPreset(preset, rawInput string) (float64, error) {
	switch preset {
	case "", formPresetDefault:
		return 0, nil
	case formPresetCPUOther:
		rawInput = strings.TrimSpace(rawInput)
		if rawInput == "" {
			return 0, usererr.New(
				"cpu is required when you pick Other",
				"type a positive fractional or whole number of cores",
			)
		}
		x, err := strconv.ParseFloat(rawInput, 64)
		if err != nil || x <= 0 {
			return 0, usererr.New(
				"cpu must be a number greater than zero",
				"omit the field or use a positive fractional or whole number of cores",
			)
		}
		return x, nil
	default:
		for _, o := range modalWizardCPUs {
			if o.Key == preset {
				return o.Value, nil
			}
		}
		return 0, usererr.New(fmt.Sprintf("invalid CPU preset %q", preset), "choose an option from the list")
	}
}

func modalMemoryFromPreset(preset, rawInput string) (int, error) {
	switch preset {
	case "", formPresetDefault:
		return 0, nil
	case formPresetMemOther:
		rawInput = strings.TrimSpace(rawInput)
		if rawInput == "" {
			return 0, usererr.New(
				"memory is required when you pick Other",
				"type a positive whole number of MiB",
			)
		}
		n, err := strconv.Atoi(rawInput)
		if err != nil || n < 1 {
			return 0, usererr.New(
				"memory must be a whole number of MiB greater than zero",
				"omit the field or pass a positive size in MiB",
			)
		}
		return n, nil
	default:
		n, err := strconv.Atoi(preset)
		if err != nil {
			return 0, usererr.New(fmt.Sprintf("invalid memory preset %q", preset), "choose an option from the list")
		}
		if slices.Contains(modalWizardMemoryMiB, n) {
			return n, nil
		}
		return 0, usererr.New(fmt.Sprintf("invalid memory preset %q", preset), "choose an option from the list")
	}
}

func resolveBlaxelWizardCompute(r *createFormResult) error {
	if r.memoryPreset == "" || r.memoryPreset == formPresetDefault {
		r.MemoryMiB = 0
		return nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(r.memoryPreset))
	if err != nil || n < 1 {
		return usererr.New(
			"memory must be a whole number of MiB greater than zero",
			"choose an option from the list",
		)
	}
	r.MemoryMiB = n
	return nil
}

func ensureWizardPresetDefaults(r *createFormResult) {
	switch r.Provider {
	case formProviderModal:
		if r.cpuPreset == "" {
			r.cpuPreset = formPresetDefault
		}
		if r.memoryPreset == "" {
			r.memoryPreset = formPresetDefault
		}
	case formProviderDocker:
		if r.dockerSizePreset == "" {
			r.dockerSizePreset = formPresetDefault
		}
	case formProviderBlaxel:
		if r.memoryPreset == "" {
			r.memoryPreset = formPresetDefault
		}
	}
}

func dockerSizeSelectOptions() []huh.Option[string] {
	opts := []huh.Option[string]{huh.NewOption("Provider default (2 CPUs, 4096 MiB)", formPresetDefault)}
	for _, s := range dockerWizardSizes {
		opts = append(opts, huh.NewOption(s.Label, s.Key))
	}
	opts = append(opts, huh.NewOption("Custom (enter CPU and memory)", formPresetDockerCustom))
	return opts
}

func modalCPUSelectOptions() []huh.Option[string] {
	opts := []huh.Option[string]{huh.NewOption("Provider default", formPresetDefault)}
	for _, o := range modalWizardCPUs {
		opts = append(opts, huh.NewOption(o.Label, o.Key))
	}
	opts = append(opts, huh.NewOption("Other (type cores)", formPresetCPUOther))
	return opts
}

func modalMemorySelectOptions() []huh.Option[string] {
	opts := []huh.Option[string]{huh.NewOption("Provider default", formPresetDefault)}
	for _, m := range modalWizardMemoryMiB {
		opts = append(opts, huh.NewOption(fmt.Sprintf("%d MiB", m), strconv.Itoa(m)))
	}
	opts = append(opts, huh.NewOption("Other (type MiB)", formPresetMemOther))
	return opts
}

func blaxelMemorySelectOptions(r *createFormResult) []huh.Option[string] {
	opts := []huh.Option[string]{huh.NewOption("Provider default (KAXA_BLAXEL_MEMORY or 4096)", formPresetDefault)}
	for _, m := range blaxelWizardMemoryMiB {
		opts = append(opts, huh.NewOption(fmt.Sprintf("%d MiB", m), strconv.Itoa(m)))
	}
	if r == nil {
		return opts
	}
	extra := strings.TrimSpace(r.memoryPreset)
	if extra == "" || extra == formPresetDefault {
		return opts
	}
	n, err := strconv.Atoi(extra)
	if err != nil || n < 1 || slices.Contains(blaxelWizardMemoryMiB, n) {
		return opts
	}
	opts = append(opts, huh.NewOption(fmt.Sprintf("%d MiB (from flags)", n), extra))
	return opts
}
