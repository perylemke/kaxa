package cli

import (
	"bytes"
	"strings"
	"testing"
)

const formModalProvider = "modal"

func TestCreateFormAccessibleAnswers(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{fixtureDemo, "1", "n", "n", "n", "n", ""}, "\n"))
	out := new(bytes.Buffer)

	got, err := collectCreateForm(in, out, []string{formModalProvider, spritesProviderName}, spritesProviderName, createFlags{})
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if got.Name != fixtureDemo || got.Provider != formModalProvider || got.Running || got.Sync || got.API || got.Public {
		t.Fatalf("result = %+v", got)
	}
	view := out.String()
	for _, title := range []string{"Name", "Provider", "Running", "Sync", "API", "Public"} {
		if !strings.Contains(view, title) {
			t.Fatalf("prompts = %q", view)
		}
	}
}

func TestCreateFormAccessibleUsesDefaults(t *testing.T) {
	in := strings.NewReader("box\n\n\n\n\n\n")
	out := new(bytes.Buffer)

	got, err := collectCreateForm(in, out, []string{formModalProvider, spritesProviderName}, spritesProviderName, createFlags{Running: true, Sync: true})
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if got.Name != "box" || got.Provider != spritesProviderName || !got.Running || !got.Sync || got.API || got.Public {
		t.Fatalf("result = %+v", got)
	}
}

func TestCreateFormAccessibleAPI(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		"box", "2", "n", "y", "y", "y", ".venv/bin/uvicorn,app.main:app", "9000", "",
	}, "\n"))
	out := new(bytes.Buffer)

	got, err := collectCreateForm(in, out, []string{formModalProvider, spritesProviderName}, spritesProviderName, createFlags{})
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if got.Provider != spritesProviderName || got.Running || !got.Sync || !got.API || !got.Public {
		t.Fatalf("result = %+v", got)
	}
	if got.APIPort != 9000 || len(got.APICmd) != 2 || got.APICmd[0] != ".venv/bin/uvicorn" {
		t.Fatalf("api = port %d cmd %#v", got.APIPort, got.APICmd)
	}
	if !strings.Contains(out.String(), "API command") || !strings.Contains(out.String(), "API port") {
		t.Fatalf("prompts = %q", out.String())
	}
}
