package blaxel

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestProviderIdentityAndCapabilities(t *testing.T) {
	t.Parallel()
	p := New()
	if p.Name() != ProviderName {
		t.Fatalf("Name = %q", p.Name())
	}
	if p.Available() {
		t.Fatal("unconfigured provider is available")
	}
	if err := p.Configure("", "kaxa"); !errors.Is(err, sandbox.ErrNotConfigured) {
		t.Fatalf("Configure missing key err = %v", err)
	}
	if err := p.Configure("key", ""); !errors.Is(err, sandbox.ErrNotConfigured) {
		t.Fatalf("Configure missing workspace err = %v", err)
	}
	if err := p.Configure("key", "kaxa"); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if !p.Available() {
		t.Fatal("configured provider is unavailable")
	}
	caps := sandbox.CapabilitiesOf(p)
	want := sandbox.Capabilities{Sync: true, Setup: true, API: true, Layout: true, Template: true}
	if caps != want {
		t.Fatalf("capabilities = %+v, want %+v", caps, want)
	}
}

func TestConfigureSameCredentialsKeepClient(t *testing.T) {
	t.Parallel()
	p := New()
	if err := p.Configure("key", "kaxa"); err != nil {
		t.Fatal(err)
	}
	first := p.api
	if err := p.Configure("key", "kaxa"); err != nil {
		t.Fatal(err)
	}
	if p.api != first {
		t.Fatal("same credentials replaced the client")
	}
	if err := p.Configure("other-key", "kaxa"); err != nil {
		t.Fatal(err)
	}
	if p.api == first {
		t.Fatal("new credentials kept the old client")
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
	api := newFakeAPI()
	api.createStatus = statusStarting
	p := newTestProvider(t, api)
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
		{"reserved 80", sandbox.Spec{Name: boxName, ExposeAPI: true, APIPort: 80}},
		{"reserved 443", sandbox.Spec{Name: boxName, ExposeAPI: true, APIPort: 443}},
		{"reserved 8080", sandbox.Spec{Name: boxName, ExposeAPI: true, APIPort: 8080}},
		{"bad port", sandbox.Spec{Name: boxName, ExposeAPI: true, APIPort: 70000}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			api := newFakeAPI()
			p := newTestProvider(t, api)
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
	api := newFakeAPI()
	p := newTestProvider(t, api)
	api.add(boxName, statusRunning)
	_, err := p.Create(t.Context(), sandbox.Spec{Name: boxName})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateTemplateRegionAndMemory(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
	if err := p.SetDefaultTemplate("blaxel/py-app:latest"); err != nil {
		t.Fatal(err)
	}
	if err := p.SetRegion("eu-lon-1"); err != nil {
		t.Fatal(err)
	}
	if err := p.SetMemory(8192); err != nil {
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
	if a.Image != "blaxel/py-app:latest" || b.Image != otherName {
		t.Fatalf("images = %q, %q", a.Image, b.Image)
	}
	if a.Region != "eu-lon-1" {
		t.Fatalf("region = %q", a.Region)
	}
	if err := p.SetDefaultTemplate("bad template"); err == nil {
		t.Fatal("invalid default template accepted")
	}
	if err := p.SetRegion("US"); err == nil {
		t.Fatal("invalid region accepted")
	}
	if err := p.SetMemory(-1); err == nil {
		t.Fatal("negative memory accepted")
	}
}

func TestCreateAPIPortOnlyWithExposeAPI(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
	_, _ = p.Create(t.Context(), sandbox.Spec{Name: "plain", APIPort: 9000})
	_, _ = p.Create(t.Context(), sandbox.Spec{Name: apiName, ExposeAPI: true, APIPort: 8000})
	plain, _ := api.Find(t.Context(), "plain")
	withAPI, _ := api.Find(t.Context(), apiName)
	if plain.APIPort != 0 || withAPI.APIPort != 8000 {
		t.Fatalf("ports = %d, %d", plain.APIPort, withAPI.APIPort)
	}
}

func TestCreateFailedSandboxIsRemoved(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	api.createStatus = statusError
	p := newTestProvider(t, api)
	if _, err := p.Create(t.Context(), sandbox.Spec{Name: boxName}); err == nil {
		t.Fatal("Create succeeded")
	}
	if len(api.sandboxes) != 0 {
		t.Fatal("failed sandbox was kept")
	}
}

func TestListSortedAndInspect(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
	api.add("zeta", statusRunning)
	api.add("alpha", statusStopped, sandbox.SyncLabel)
	infos, err := p.List(t.Context())
	if err != nil || len(infos) != 2 || infos[0].ID != "alpha" || infos[0].Status != statusStopped {
		t.Fatalf("List = %+v, %v", infos, err)
	}
	d, err := p.Inspect(t.Context(), "alpha")
	if err != nil || d.Provider != ProviderName || d.URL != "" {
		t.Fatalf("Inspect = %+v, %v", d, err)
	}
	if !fieldHasLabel(d.Fields, "Workspace") {
		t.Fatalf("fields = %+v", d.Fields)
	}
	if _, err := p.Inspect(t.Context(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Inspect missing err = %v", err)
	}
}

func TestInspectReportsPublishedAPIURL(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
	if _, err := p.Create(t.Context(), sandbox.Spec{Name: apiName, ExposeAPI: true, APIPort: 8000}); err != nil {
		t.Fatal(err)
	}
	d, err := p.Inspect(t.Context(), apiName)
	if err != nil || d.URL != "" {
		t.Fatalf("URL before publish = %+v, %v", d, err)
	}
	if _, err := api.CreatePreview(t.Context(), apiName, 8000); err != nil {
		t.Fatal(err)
	}
	d, err = p.Inspect(t.Context(), apiName)
	if err != nil || d.URL == "" || d.Auth != sandbox.URLAuthPublic {
		t.Fatalf("Inspect = %+v, %v", d, err)
	}
}

func TestDestroy(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
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

func TestDestroyWaitsUntilNameIsGone(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	api.lingerAfterDelete = 2
	p := newTestProvider(t, api)
	api.add(boxName, statusRunning)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := p.Destroy(ctx, boxName); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if _, err := api.Find(t.Context(), boxName); !errors.Is(err, ErrNotFound) {
		t.Fatalf("sandbox still present: %v", err)
	}
}

func TestDestroyAcceptsTerminatedName(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	api.terminateAfterDelete = true
	p := newTestProvider(t, api)
	api.add(boxName, statusRunning)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := p.Destroy(ctx, boxName); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	rec, err := api.Find(t.Context(), boxName)
	if err != nil || rec.Phase != phaseTerminated {
		t.Fatalf("record = %+v, %v", rec, err)
	}
	if _, err := p.Create(t.Context(), sandbox.Spec{Name: boxName}); err != nil {
		t.Fatalf("Create over terminated: %v", err)
	}
	api.add("failed", statusError)
	api.sandboxes["failed"].Phase = phaseFailed
	if _, err := p.Create(t.Context(), sandbox.Spec{Name: "failed"}); err != nil {
		t.Fatalf("Create over failed: %v", err)
	}
}

func TestDestroyReportsNameThatStays(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	api.lingerAfterDelete = -1
	p := newTestProvider(t, api)
	p.SetGoneTimeout(30 * time.Millisecond)
	api.add(boxName, statusRunning)
	err := p.Destroy(t.Context(), boxName)
	if err == nil || !strings.Contains(err.Error(), boxName) || !strings.Contains(err.Error(), "still exists") {
		t.Fatalf("err = %v", err)
	}
	rec, findErr := api.Find(t.Context(), boxName)
	if findErr != nil || rec.Phase != phaseDeleting {
		t.Fatalf("record = %+v, %v", rec, findErr)
	}
}

func TestDefaults(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
	if _, err := p.Create(t.Context(), sandbox.Spec{Name: boxName}); err != nil {
		t.Fatal(err)
	}
	rec, err := api.Find(t.Context(), boxName)
	if err != nil || rec.Image != defaultImage || rec.Region != defaultRegion {
		t.Fatalf("record = %+v, %v", rec, err)
	}
}

func fieldHasLabel(fields []sandbox.Field, label string) bool {
	for _, f := range fields {
		if f.Label == label {
			return true
		}
	}
	return false
}
