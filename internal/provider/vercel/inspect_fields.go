package vercel

import (
	"strings"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func vercelDetailFields(rec Record, url, auth string) []sandbox.Field {
	var fields []sandbox.Field
	fields = sandbox.AppendTextField(fields, "Region", rec.Region)
	fields = sandbox.AppendTextField(fields, "Runtime", rec.Runtime)
	fields = sandbox.AppendTextField(fields, "Image", rec.Image)
	fields = sandbox.AppendIntField(fields, "CPU", rec.VCPUs)
	fields = sandbox.AppendIntField(fields, "Memory", rec.MemoryMB)
	fields = sandbox.AppendTextField(fields, "Snapshot", rec.SnapshotID)
	fields = sandbox.AppendTimeField(fields, "Ends", rec.ExpiresAt)
	fields = sandbox.AppendTextField(fields, "URL", url)
	fields = sandbox.AppendTextField(fields, "Auth", auth)
	return fields
}

func cutEnv(entry string) (key, value string, ok bool) {
	return strings.Cut(entry, "=")
}
