package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestCreateFormAccessibleAnswers(t *testing.T) {
	in := strings.NewReader("demo\n1\nn\n")
	out := new(bytes.Buffer)

	got, err := collectCreateForm(in, out, []string{"modal", "sprites"}, "sprites", false)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if got.Name != "demo" || got.Provider != "modal" || got.Running {
		t.Fatalf("result = %+v", got)
	}
	view := out.String()
	if !strings.Contains(view, "Name") || !strings.Contains(view, "Provider") || !strings.Contains(view, "Running") {
		t.Fatalf("prompts = %q", view)
	}
}

func TestCreateFormAccessibleUsesDefaults(t *testing.T) {
	in := strings.NewReader("box\n\n\n")
	out := new(bytes.Buffer)

	got, err := collectCreateForm(in, out, []string{"modal", "sprites"}, "sprites", true)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if got.Name != "box" || got.Provider != "sprites" || !got.Running {
		t.Fatalf("result = %+v", got)
	}
}
