package e2b

import "github.com/perylemke/kaxa/internal/project"

// The base template has 512 MiB of memory, and E2B sets that per template, not
// per sandbox. Compiling a large package, such as generated protobuf code, is
// killed by the kernel there. Setup therefore only downloads Go and Rust
// dependencies; the first build runs later, under the user's control.

// stepsFor keeps the steps as they are for a template of the user's choice,
// whose size is the user's to pick, and makes them download-only on the base
// template.
func stepsFor(rec Record, steps []project.Step) []project.Step {
	if rec.Template != "" {
		return steps
	}
	return downloadOnly(steps)
}

// downloadOnly returns steps with the compiling commands of Go and Rust
// replaced by their download-only equivalents. The input is not modified.
func downloadOnly(steps []project.Step) []project.Step {
	out := make([]project.Step, len(steps))
	for i, step := range steps {
		out[i] = project.Step{Kind: step.Kind, Commands: make([][]string, len(step.Commands))}
		for j, argv := range step.Commands {
			out[i].Commands[j] = downloadArgv(step.Kind, argv)
		}
	}
	return out
}

func downloadArgv(kind project.Kind, argv []string) []string {
	if len(argv) < 2 || argv[1] != "build" {
		return append([]string(nil), argv...)
	}
	if kind == project.KindGo {
		return []string{argv[0], "mod", "download"}
	}
	if kind == project.KindRust {
		return append([]string{argv[0], "fetch"}, argv[2:]...)
	}
	return append([]string(nil), argv...)
}
