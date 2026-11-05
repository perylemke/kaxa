package docker

import (
	"fmt"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func dockerDetailFields(rec Record) []sandbox.Field {
	var fields []sandbox.Field
	fields = sandbox.AppendTextField(fields, "Image", rec.Image)
	fields = sandbox.AppendTimeField(fields, "Created", rec.CreatedAt)
	fields = sandbox.AppendTextField(fields, "Failure", rec.Failure)
	fields = sandbox.AppendIntField(fields, "CPUs", rec.CPUs)
	if rec.MemoryMiB > 0 {
		fields = sandbox.AppendFormattedField(fields, "Memory", "%d MiB", rec.MemoryMiB)
	}
	for _, p := range rec.Ports {
		if p.URL == "" {
			continue
		}
		label := fmt.Sprintf("Port %d", p.Port)
		fields = sandbox.AppendTextField(fields, label, p.URL)
	}
	return fields
}
