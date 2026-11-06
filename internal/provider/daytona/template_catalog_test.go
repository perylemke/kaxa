package daytona

import (
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestListTemplatesKeepsActiveGeneralSnapshots(t *testing.T) {
	t.Parallel()

	api := newFakeAPI()
	api.catalog = []snapshotCatalogRow{
		{Name: "mine", General: false, State: snapshotStateActive, CPU: 1, Memory: 1, Disk: 1},
		{Name: "old", General: true, State: "inactive", CPU: 1, Memory: 1, Disk: 1},
		{Name: "", General: true, State: snapshotStateActive, CPU: 1, Memory: 1, Disk: 1},
		{Name: "daytona-small", General: true, State: snapshotStateActive, CPU: 1, Memory: 1, Disk: 3},
		{Name: "daytona-medium", General: true, State: snapshotStateActive, CPU: 2, Memory: 4, Disk: 8},
	}
	p := NewWithClients(api, nil, "key")

	got, err := p.ListTemplates(t.Context())
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	want := []sandbox.TemplateOption{
		{Name: "daytona-medium", Label: "daytona-medium (2 CPUs, 4 GB memory, 8 GB disk)"},
		{Name: "daytona-small", Label: "daytona-small (1 CPU, 1 GB memory, 3 GB disk)"},
	}
	expect(t, "templates", got, want)
}

func TestListTemplatesRequiresCredentials(t *testing.T) {
	t.Parallel()

	if _, err := New().ListTemplates(t.Context()); err == nil {
		t.Fatal("expected missing credentials")
	}
}
