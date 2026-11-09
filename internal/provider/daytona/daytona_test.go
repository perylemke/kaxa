package daytona

import (
	"errors"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestProviderIdentityAndCapabilities(t *testing.T) {
	t.Parallel()
	p := New()
	if p.Name() != "daytona" {
		t.Fatalf("Name = %q", p.Name())
	}
	if p.Available() {
		t.Fatal("unconfigured provider is available")
	}
	if err := p.Configure(""); !errors.Is(err, sandbox.ErrNotConfigured) {
		t.Fatalf("Configure empty err = %v", err)
	}
	if err := p.Configure("key"); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if !p.Available() {
		t.Fatal("configured provider is unavailable")
	}
	caps := sandbox.CapabilitiesOf(p)
	want := sandbox.Capabilities{
		Sync: true, Setup: true, API: true, Layout: true, Restart: true, Template: true,
		NetworkBlock: true, Snapshot: true, SnapshotRestore: string(sandbox.RestoreNewSandbox),
	}
	if caps != want {
		t.Fatalf("capabilities = %+v, want %+v", caps, want)
	}
}

func TestConfigureSameKeyKeepsClient(t *testing.T) {
	t.Parallel()
	p := New()
	_ = p.Configure("key")
	first := p.api
	_ = p.Configure("key")
	if p.api != first {
		t.Fatal("same key replaced the client")
	}
	_ = p.Configure(otherName)
	if p.api == first {
		t.Fatal("new key kept the old client")
	}
}

func TestUnconfiguredProviderFails(t *testing.T) {
	t.Parallel()
	p := New()
	if _, err := p.List(t.Context()); !errors.Is(err, sandbox.ErrNotConfigured) {
		t.Fatalf("List err = %v", err)
	}
	if _, err := p.Create(t.Context(), sandbox.Spec{Name: boxName}); !errors.Is(err, sandbox.ErrNotConfigured) {
		t.Fatalf("Create err = %v", err)
	}
}

func TestCreateWaitsUntilRunning(t *testing.T) {
	t.Parallel()
	p, api := newTestProvider(t, newFakeToolbox(okHandler))
	api.createStatus = "queued"
	id, err := p.Create(t.Context(), sandbox.Spec{Name: boxName, Labels: []string{sandbox.SyncLabel}})
	if err != nil || id != boxName {
		t.Fatalf("Create = %q, %v", id, err)
	}
	rec, err := api.Find(t.Context(), boxName)
	if err != nil || rec.Status != statusRunning || len(rec.Labels) != 1 {
		t.Fatalf("record = %+v, %v", rec, err)
	}
}

func TestCreateValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		spec sandbox.Spec
	}{
		{"uppercase name", sandbox.Spec{Name: "Box"}},
		{"empty name", sandbox.Spec{}},
		{"bad template", sandbox.Spec{Name: boxName, Template: "../x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, api := newTestProvider(t, newFakeToolbox(okHandler))
			if _, err := p.Create(t.Context(), tt.spec); err == nil {
				t.Fatal("Create succeeded")
			}
			if len(api.sandboxes) != 0 {
				t.Fatal("a sandbox was created")
			}
		})
	}
}

func TestCreateRejectsDuplicate(t *testing.T) {
	t.Parallel()
	p, api := newTestProvider(t, newFakeToolbox(okHandler))
	api.add(boxName, statusRunning)
	_, err := p.Create(t.Context(), sandbox.Spec{Name: boxName})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateTemplatePrecedence(t *testing.T) {
	t.Parallel()
	p, api := newTestProvider(t, newFakeToolbox(okHandler))
	if err := p.SetDefaultTemplate("team-img"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Create(t.Context(), sandbox.Spec{Name: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Create(t.Context(), sandbox.Spec{Name: "b", Template: otherName}); err != nil {
		t.Fatal(err)
	}
	a, _ := api.Find(t.Context(), "a")
	b, _ := api.Find(t.Context(), "b")
	if a.Template != "team-img" || b.Template != otherName {
		t.Fatalf("templates = %q, %q", a.Template, b.Template)
	}
	if err := p.SetDefaultTemplate("bad template"); err == nil {
		t.Fatal("invalid default template accepted")
	}
}

func TestCreateAPIPortOnlyWithExposeAPI(t *testing.T) {
	t.Parallel()
	p, api := newTestProvider(t, newFakeToolbox(okHandler))
	_, _ = p.Create(t.Context(), sandbox.Spec{Name: "plain", APIPort: 9000})
	_, _ = p.Create(t.Context(), sandbox.Spec{Name: "api", ExposeAPI: true, APIPort: 8000})
	plain, _ := api.Find(t.Context(), "plain")
	withAPI, _ := api.Find(t.Context(), "api")
	if plain.APIPort != 0 || withAPI.APIPort != 8000 {
		t.Fatalf("ports = %d, %d", plain.APIPort, withAPI.APIPort)
	}
}

func TestCreatePassesNetworkBlock(t *testing.T) {
	t.Parallel()
	p, api := newTestProvider(t, newFakeToolbox(okHandler))
	if _, err := p.Create(t.Context(), sandbox.Spec{Name: "open"}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Create(t.Context(), sandbox.Spec{Name: "closed", NetworkBlockAll: true}); err != nil {
		t.Fatal(err)
	}
	open, _ := api.Find(t.Context(), "open")
	closed, _ := api.Find(t.Context(), "closed")
	if open.NetworkBlockAll || !closed.NetworkBlockAll {
		t.Fatalf("network block = %v, %v", open.NetworkBlockAll, closed.NetworkBlockAll)
	}
}

func TestCreateFailedSandboxIsRemoved(t *testing.T) {
	t.Parallel()
	p, api := newTestProvider(t, newFakeToolbox(okHandler))
	api.createStatus = statusFailed
	if _, err := p.Create(t.Context(), sandbox.Spec{Name: boxName}); err == nil {
		t.Fatal("Create succeeded")
	}
	if len(api.sandboxes) != 0 {
		t.Fatal("failed sandbox was kept")
	}
}

func TestListSortedAndInspect(t *testing.T) {
	t.Parallel()
	p, api := newTestProvider(t, newFakeToolbox(okHandler))
	api.add("zeta", statusRunning)
	api.add("alpha", statusStopped, sandbox.SyncLabel)
	infos, err := p.List(t.Context())
	if err != nil || len(infos) != 2 || infos[0].ID != "alpha" || infos[0].Status != statusStopped {
		t.Fatalf("List = %+v, %v", infos, err)
	}
	d, err := p.Inspect(t.Context(), "alpha")
	if err != nil || d.Provider != "daytona" || d.URL != "" {
		t.Fatalf("Inspect = %+v, %v", d, err)
	}
	if len(d.Fields) == 0 || d.Fields[0].Label != "ID" {
		t.Fatalf("fields = %+v", d.Fields)
	}
	if _, err := p.Inspect(t.Context(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Inspect missing err = %v", err)
	}
}

func TestInspectReportsPublishedAPIURL(t *testing.T) {
	t.Parallel()
	p, api := newTestProvider(t, newFakeToolbox(okHandler))
	if _, err := p.Create(t.Context(), sandbox.Spec{Name: "api", ExposeAPI: true, APIPort: 8000}); err != nil {
		t.Fatal(err)
	}
	d, _ := p.Inspect(t.Context(), "api")
	if d.URL != "" {
		t.Fatalf("URL before publish = %q", d.URL)
	}
	rec, _ := api.Find(t.Context(), "api")
	_, _ = api.SetPort(t.Context(), rec.ID, 8000, true)
	d, _ = p.Inspect(t.Context(), "api")
	if d.URL == "" || d.Auth != sandbox.URLAuthPublic {
		t.Fatalf("Inspect = %+v", d)
	}
}

func TestDestroy(t *testing.T) {
	t.Parallel()
	p, api := newTestProvider(t, newFakeToolbox(okHandler))
	api.add(boxName, statusStopped)
	if err := p.Destroy(t.Context(), boxName); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if err := p.Destroy(t.Context(), boxName); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Destroy err = %v", err)
	}
	if err := p.Destroy(t.Context(), "Bad Name"); err == nil {
		t.Fatal("invalid name accepted")
	}
}
