package web

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// imaged is a provider that starts from an image and publishes a catalog.
type imaged struct {
	*fakeProvider
	options []sandbox.TemplateOption
	err     error
}

func (imaged) SupportsTemplate() {}

func (i imaged) ListTemplates(context.Context) ([]sandbox.TemplateOption, error) {
	return i.options, i.err
}

// imageOnly starts from an image but has no catalog.
type imageOnly struct{ *fakeProvider }

func (imageOnly) SupportsTemplate() {}

type templatesOut struct {
	Templates []templateJSON `json:"templates"`
	Warning   string         `json:"warning"`
}

func TestListTemplates(t *testing.T) {
	t.Parallel()

	catalog := []sandbox.TemplateOption{{Name: "img:1", Label: "Image one"}, {Name: "img:2"}}
	cases := []struct {
		name        string
		p           sandbox.Provider
		want        []templateJSON
		wantWarning string
	}{
		{name: "catalog", p: imaged{fakeProvider: &fakeProvider{name: testProvider}, options: catalog}, want: []templateJSON{{Name: "img:1", Label: "Image one"}, {Name: "img:2"}}},
		{name: "no catalog", p: imageOnly{&fakeProvider{name: testProvider}}},
		{name: "no template support", p: &fakeProvider{name: testProvider}},
		{name: "catalog fails", p: imaged{fakeProvider: &fakeProvider{name: testProvider}, err: stringError("hub down")}, wantWarning: "hub down"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, h, stderr := loggedServer(t, tt.p, Options{})
			rec := do(t, h, s, http.MethodGet, "/api/providers/lab/templates", nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("code = %d %s", rec.Code, rec.Body)
			}
			var got templatesOut
			decode(t, rec, &got)
			if len(got.Templates) != len(tt.want) || got.Warning != tt.wantWarning {
				t.Fatalf("got %+v, want %v warning %q", got, tt.want, tt.wantWarning)
			}
			for i, want := range tt.want {
				if got.Templates[i] != want {
					t.Fatalf("template %d = %+v, want %+v", i, got.Templates[i], want)
				}
			}
			if strings.Contains(rec.Body.String(), "null") {
				t.Fatalf("templates must be a list, got %s", rec.Body)
			}
			if tt.wantWarning != "" && !strings.Contains(stderr.String(), "warn   templates") {
				t.Fatalf("log\n%s", stderr.String())
			}
		})
	}
}

func TestListTemplatesRefuses(t *testing.T) {
	t.Parallel()

	s, h := newServer(t, &fakeProvider{name: testProvider}, Options{Provider: testProvider})
	cases := map[string]string{
		"bad name":         "/api/providers/BAD_NAME/templates",
		"outside session":  "/api/providers/" + testOther + "/templates",
		"unknown provider": "/api/providers/nope/templates",
	}
	for name, path := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if rec := do(t, h, s, http.MethodGet, path, nil); rec.Code < 400 || rec.Code >= 500 {
				t.Fatalf("code = %d %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestCreateStartsFromTheImage(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testProvider}
	s, h, stderr := loggedServer(t, imageOnly{fp}, Options{})
	rec := do(t, h, s, http.MethodPost, "/api/sandboxes", createBody{Name: testSandbox, Provider: testProvider, Template: "  reg/app:1  "})
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d %s", rec.Code, rec.Body)
	}
	if fp.created.Template != "reg/app:1" {
		t.Fatalf("template = %q", fp.created.Template)
	}
	if !strings.Contains(stderr.String(), "image reg/app:1") {
		t.Fatalf("log\n%s", stderr.String())
	}
}

func TestCreateRefusesAnImage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		p        sandbox.Provider
		template string
	}{
		{name: "provider cannot", p: &fakeProvider{name: testProvider}, template: "reg/app:1"},
		{name: "too long", p: imageOnly{&fakeProvider{name: testProvider}}, template: strings.Repeat("a", maxTemplateLen+1)},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s, h := newServer(t, tt.p, Options{})
			rec := do(t, h, s, http.MethodPost, "/api/sandboxes", createBody{Name: testSandbox, Provider: testProvider, Template: tt.template})
			if rec.Code < 400 || rec.Code >= 500 {
				t.Fatalf("code = %d %s", rec.Code, rec.Body)
			}
		})
	}
}
