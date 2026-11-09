package blaxel

import (
	"slices"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestListTemplatesKeepsVisibleHubImages(t *testing.T) {
	t.Parallel()

	api := newFakeAPI()
	api.hub = []hubTemplate{
		{Image: "blaxel/hidden:latest", DisplayName: "Hidden", Memory: 4096, Hidden: true},
		{Image: "blaxel/soon:latest", DisplayName: "Soon", Memory: 4096, ComingSoon: true},
		{DisplayName: "No image", Memory: 4096},
		{Image: "blaxel/py-app:latest", DisplayName: "Python", Memory: 4096},
		{Image: "blaxel/base-image:latest", DisplayName: "Base", Memory: 4096},
		{Image: "blaxel/lightpanda:latest", Memory: 1024},
		{Image: "blaxel/plain:latest", DisplayName: "Plain"},
	}
	p := NewWithClients(api, api, "key", "kaxa")

	got, err := p.ListTemplates(t.Context())
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	want := []sandbox.TemplateOption{
		{Name: "blaxel/base-image:latest", Label: "Base (blaxel/base-image:latest, 4096 MB)"},
		{Name: "blaxel/lightpanda:latest", Label: "blaxel/lightpanda:latest (1024 MB)"},
		{Name: "blaxel/plain:latest", Label: "Plain (blaxel/plain:latest)"},
		{Name: "blaxel/py-app:latest", Label: "Python (blaxel/py-app:latest, 4096 MB)"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("templates = %#v, want %#v", got, want)
	}
}

func TestListTemplatesRequiresCredentials(t *testing.T) {
	t.Parallel()

	if _, err := New().ListTemplates(t.Context()); err == nil {
		t.Fatal("expected missing credentials")
	}
}
