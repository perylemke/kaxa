package e2b

import (
	"strconv"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func e2bDetailFields(rec Record, url, auth string) []sandbox.Field {
	var fields []sandbox.Field
	fields = sandbox.AppendTextField(fields, "ID", rec.ID)
	template := rec.ListTemplateID
	if template == "" {
		template = rec.Template
	}
	fields = sandbox.AppendTextField(fields, "Template", template)
	fields = sandbox.AppendIntField(fields, "CPU", rec.CPUCount)
	fields = sandbox.AppendIntField(fields, "Memory", rec.MemoryMB)
	fields = sandbox.AppendIntField(fields, "Disk", rec.DiskSizeMB)
	fields = sandbox.AppendTimeField(fields, "Ends", rec.EndAt)
	fields = sandbox.AppendTextField(fields, "Domain", rec.Domain)
	fields = sandbox.AppendTextField(fields, "URL", url)
	fields = sandbox.AppendTextField(fields, "Auth", auth)
	return fields
}

func e2bPublicURL(rec Record, domain string) (string, string) {
	if rec.APIPort <= 0 {
		return "", ""
	}
	host := domain
	if rec.Domain != "" {
		host = rec.Domain
	}
	return "https://" + strconv.Itoa(rec.APIPort) + "-" + rec.ID + "." + host, sandbox.URLAuthPublic
}
