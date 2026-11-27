package web

import (
	"net/http"
	"testing"

	"github.com/perylemke/kaxa/internal/provider/vercel"
	"github.com/perylemke/kaxa/internal/registry"
)

func TestProvidersPublishComputeOptions(t *testing.T) {
	t.Parallel()

	plain := &fakeProvider{name: testProvider}
	big := sized{capable{&fakeProvider{name: testOther}}}
	reg := registry.New()
	mustRegister(t, reg, plain, big)
	s, h := newServer(t, nil, Options{Registry: reg})
	rec := do(t, h, s, http.MethodGet, "/api/providers", nil)
	var payload struct {
		Providers []providerInfo `json:"providers"`
	}
	decode(t, rec, &payload)
	if len(payload.Providers) != 2 {
		t.Fatalf("providers = %+v", payload.Providers)
	}
	if payload.Providers[0].Name != testProvider || payload.Providers[0].Compute != nil {
		t.Fatalf("plain = %+v", payload.Providers[0])
	}
	got := payload.Providers[1].Compute
	if got == nil || len(got.CPUs) != 2 || got.MemoryMiB[0] != 1024 || !got.CustomMemory || !payload.Providers[1].Capabilities.CPU {
		t.Fatalf("sized = %+v", payload.Providers[1])
	}
}

func TestCreatePassesCompute(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testProvider}
	s, h := newServer(t, sized{capable{fp}}, Options{})
	rec := do(t, h, s, http.MethodPost, "/api/sandboxes", createBody{
		Name: testSandbox, Provider: testProvider, CPU: 1.5, MemoryMiB: 2048, DiskPercent: 60,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	if fp.created.CPU != 1.5 || fp.created.MemoryMiB != 2048 || fp.created.DiskPercent != 60 {
		t.Fatalf("spec = %+v", fp.created)
	}
}

func TestCreateRefusesBadCompute(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testProvider}
	s, h := newServer(t, sized{capable{fp}}, Options{})
	plain := &fakeProvider{name: testOther}
	ps, ph := newServer(t, plain, Options{})
	cases := []struct {
		name string
		srv  *Server
		h    http.Handler
		body createBody
	}{
		{name: "negative cpu", srv: s, h: h, body: createBody{Name: testSandbox, Provider: testProvider, CPU: -1}},
		{name: "negative memory", srv: s, h: h, body: createBody{Name: testSandbox, Provider: testProvider, MemoryMiB: -1}},
		{name: "disk percent too low", srv: s, h: h, body: createBody{Name: testSandbox, Provider: testProvider, DiskPercent: 5}},
		{name: "cpu on a provider without it", srv: ps, h: ph, body: createBody{Name: testSandbox, Provider: testOther, CPU: 2}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, tt.h, tt.srv, http.MethodPost, "/api/sandboxes", tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestVercelPublishesComputeOptions(t *testing.T) {
	t.Parallel()

	reg := registry.New()
	mustRegister(t, reg, vercel.New())
	s, h := newServer(t, nil, Options{Registry: reg})
	rec := do(t, h, s, http.MethodGet, "/api/providers", nil)
	var payload struct {
		Providers []providerInfo `json:"providers"`
	}
	decode(t, rec, &payload)
	if len(payload.Providers) != 1 {
		t.Fatalf("providers = %+v", payload.Providers)
	}
	got := payload.Providers[0]
	if got.Compute == nil || len(got.Compute.CPUs) != 4 || !got.Compute.CustomCPU || len(got.Compute.MemoryMiB) != 0 || got.Compute.CustomMemory {
		t.Fatalf("compute = %+v, want 1, 2, 4, 8 vCPUs and no memory choice", got.Compute)
	}
	if !got.Capabilities.Template {
		t.Fatalf("capabilities = %+v, want the image field", got.Capabilities)
	}
}
