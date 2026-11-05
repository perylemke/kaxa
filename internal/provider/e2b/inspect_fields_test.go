package e2b

import (
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestE2BDetailFieldsFromListItem(t *testing.T) {
	t.Parallel()

	cpu := 2
	mem := 512
	item := listedSandbox{
		SandboxID:  "sbx-1",
		StartedAt:  time.Now(),
		State:      "running",
		Metadata:   map[string]string{metaManaged: metaTrue, metaName: "box", metaTemplate: "custom"},
		TemplateID: "tpl-9",
		CPUCount:   &cpu,
		MemoryMB:   &mem,
	}
	rec, ok := recordOf(item)
	if !ok {
		t.Fatal("recordOf failed")
	}
	fields := e2bDetailFields(rec, "", "")
	text := joinFieldText(fields)
	for _, want := range []string{"sbx-1", "tpl-9", "CPU", "512"} {
		if !strings.Contains(text, want) {
			t.Fatalf("fields missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, "envdAccessToken") || strings.Contains(text, "secret") {
		t.Fatalf("fields leaked token: %s", text)
	}
}

func joinFieldText(fields []sandbox.Field) string {
	var b strings.Builder
	for _, f := range fields {
		b.WriteString(f.Label)
		b.WriteString(f.Text)
	}
	return b.String()
}
