package docker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
)

const (
	tagGemini = "gemini"
	tagShell  = "shell"
)

func hubImage(tag string) string {
	return templateRepo + ":" + tag
}

func TestTemplateOptionsKeepsFloatingTags(t *testing.T) {
	t.Parallel()

	got := templateOptions([]string{
		"claude-code-nightly",
		"latest",
		"claude-code-0.11.0",
		"0.11.0",
		"",
		"../shell",
		tagGemini,
		tagShell,
		"claude-code",
	})
	want := []sandbox.TemplateOption{
		{Name: "docker/sandbox-templates:claude-code", Label: "claude-code"},
		{Name: hubImage(tagGemini), Label: tagGemini},
		{Name: hubImage(tagShell), Label: tagShell},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("templates = %#v, want %#v", got, want)
	}
}

type fakeTags struct {
	names []string
	err   error
}

func (f fakeTags) ListTags(context.Context) ([]string, error) {
	return f.names, f.err
}

func TestListTemplatesRequiresCredentials(t *testing.T) {
	t.Parallel()

	if _, err := New().ListTemplates(t.Context()); err == nil {
		t.Fatal("expected missing credentials")
	}
}

func TestListTemplatesUsesFloatingTags(t *testing.T) {
	t.Parallel()

	p := NewWithClients(newFakeAPI(), &fakeStarter{}, time.Millisecond, time.Millisecond)
	p.tags = fakeTags{names: []string{tagShell, "latest", "claude-code-nightly", tagGemini}}

	got, err := p.ListTemplates(t.Context())
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	want := []sandbox.TemplateOption{
		{Name: hubImage(tagGemini), Label: tagGemini},
		{Name: hubImage(tagShell), Label: tagShell},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("templates = %#v, want %#v", got, want)
	}
}

func TestHubClientStopsAfterLastPage(t *testing.T) {
	t.Parallel()

	var pages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page_size") != "100" {
			t.Errorf("page_size = %q", r.URL.Query().Get("page_size"))
		}
		pages = append(pages, r.URL.Query().Get("page"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("page") {
		case "1":
			_, _ = w.Write([]byte(`{"next":"https://hub.docker.com/v2/next","results":[{"name":"shell"},{"name":"latest"}]}`))
		case "2":
			_, _ = w.Write([]byte(`{"next":null,"results":[{"name":"gemini"},{"name":"claude-code-0.11.0"}]}`))
		default:
			t.Errorf("unexpected page %q", r.URL.Query().Get("page"))
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(srv.Close)

	c := &hubClient{base: srv.URL, http: hubHTTPClient()}
	got, err := c.ListTags(t.Context())
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	want := []string{tagShell, "latest", tagGemini, "claude-code-0.11.0"}
	if !slices.Equal(got, want) {
		t.Fatalf("tags = %#v, want %#v", got, want)
	}
	if !slices.Equal(pages, []string{"1", "2"}) {
		t.Fatalf("pages = %#v", pages)
	}
}

func TestHubClientRefusesRedirect(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.com/tags", http.StatusFound)
	}))
	t.Cleanup(srv.Close)

	c := &hubClient{base: srv.URL, http: hubHTTPClient()}
	if _, err := c.ListTags(t.Context()); err == nil {
		t.Fatal("expected redirect to fail")
	}
}
