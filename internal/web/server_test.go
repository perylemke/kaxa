package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/config"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestCreateInspectAndDestroy(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{
		name: testProvider,
		list: []sandbox.Info{{ID: testSandbox, Provider: testProvider, Status: testStatus}},
		detail: sandbox.Detail{
			ID: testSandbox, Provider: testProvider, Status: testStatus, URL: "https://example.test/box", Auth: sandbox.URLAuthSprite,
			CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			Fields:    []sandbox.Field{{Label: "Region", Text: "iad"}, {Label: "Empty"}, {Label: "When", At: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)}},
		},
	}
	s, h := newServer(t, fp, Options{})

	created := do(t, h, s, http.MethodPost, "/api/sandboxes", createBody{Name: testSandbox, Provider: testProvider})
	if created.Code != http.StatusOK {
		t.Fatalf("create = %d %s", created.Code, created.Body)
	}
	out := createDone(t, created)
	if out.ID != testSandbox || out.URL != "https://example.test/box" || fp.created.Name != testSandbox {
		t.Fatalf("created = %+v spec %+v", out, fp.created)
	}

	got := do(t, h, s, http.MethodGet, "/api/sandboxes/lab/box", nil)
	if got.Code != http.StatusOK {
		t.Fatalf("inspect = %d %s", got.Code, got.Body)
	}
	var detail detailJSON
	decode(t, got, &detail)
	if detail.Auth != sandbox.URLAuthSprite || detail.CreatedAt == "" || len(detail.Fields) != 2 {
		t.Fatalf("detail = %+v", detail)
	}

	listed := do(t, h, s, http.MethodGet, "/api/sandboxes", nil)
	if listed.Code != http.StatusOK || !bytes.Contains(listed.Body.Bytes(), []byte(`"id":"box"`)) {
		t.Fatalf("list = %d %s", listed.Code, listed.Body)
	}
}

func TestDestroyRequiresTheSandboxName(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testProvider}
	s, h := newServer(t, fp, Options{})
	denied := do(t, h, s, http.MethodPost, "/api/sandboxes/lab/box/destroy", destroyBody{Confirm: testOther})
	if denied.Code != http.StatusBadRequest || fp.destroyed != "" {
		t.Fatalf("mismatch = %d destroyed %q", denied.Code, fp.destroyed)
	}
	destroyed := do(t, h, s, http.MethodPost, "/api/sandboxes/lab/box/destroy", destroyBody{Confirm: testSandbox})
	if destroyed.Code != http.StatusOK || fp.destroyed != testSandbox {
		t.Fatalf("destroy = %d %q", destroyed.Code, fp.destroyed)
	}
}

func TestCreateValidation(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testProvider}
	s, h := newServer(t, fp, Options{})
	tests := []struct {
		name string
		body createBody
	}{
		{name: "bad name", body: createBody{Name: "Bad", Provider: testProvider}},
		{name: "dir without sync", body: createBody{Name: testSandbox, Provider: testProvider, Dir: "/tmp"}},
		{name: "api without command", body: createBody{Name: testSandbox, Provider: testProvider, API: true}},
		{name: "running unsupported", body: createBody{Name: testSandbox, Provider: testProvider, Running: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(t, h, s, http.MethodPost, "/api/sandboxes", tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestInspectNotFound(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testProvider, inspectErr: stringError("sandbox box not found")}
	s, h := newServer(t, fp, Options{})
	rec := do(t, h, s, http.MethodGet, "/api/sandboxes/lab/box", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
}

func TestActionsFollowCapabilities(t *testing.T) {
	t.Parallel()

	plain := &fakeProvider{name: testProvider}
	s, h := newServer(t, plain, Options{})
	restart := do(t, h, s, http.MethodPost, "/api/sandboxes/lab/box/restart", map[string]string{})
	if restart.Code != http.StatusBadRequest {
		t.Fatalf("restart = %d %s", restart.Code, restart.Body)
	}

	full := &fakeProvider{name: testProvider, preview: "preview-once"}
	cs, ch := newServer(t, capable{full}, Options{})
	if rec := do(t, ch, cs, http.MethodPost, "/api/sandboxes/lab/box/restart", map[string]string{}); rec.Code != http.StatusOK || full.restarted != testSandbox {
		t.Fatalf("restart = %d %q %s", rec.Code, full.restarted, rec.Body)
	}
	if rec := do(t, ch, cs, http.MethodPost, "/api/sandboxes/lab/box/upgrade", map[string]string{}); rec.Code != http.StatusOK || full.upgraded != testSandbox {
		t.Fatalf("upgrade = %d %q", rec.Code, full.upgraded)
	}
	auth := do(t, ch, cs, http.MethodPost, "/api/sandboxes/lab/box/url-auth", urlAuthBody{Auth: "public"})
	if auth.Code != http.StatusOK || full.auth != sandbox.URLAuthPublic {
		t.Fatalf("auth = %d %q %s", auth.Code, full.auth, auth.Body)
	}
	bad := do(t, ch, cs, http.MethodPost, "/api/sandboxes/lab/box/url-auth", urlAuthBody{Auth: "open"})
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad auth = %d %s", bad.Code, bad.Body)
	}
	created := do(t, ch, cs, http.MethodPost, "/api/sandboxes", createBody{Name: testSandbox, Provider: testProvider})
	if created.Code != http.StatusOK || !bytes.Contains(created.Body.Bytes(), []byte("preview-once")) {
		t.Fatalf("preview = %d %s", created.Code, created.Body)
	}
}

func TestProviderListMarksUnavailable(t *testing.T) {
	t.Parallel()

	lab := &fakeProvider{name: testProvider}
	off := &fakeProvider{name: "off", unavailable: true}
	reg := registry.New()
	mustRegister(t, reg, lab, off)
	s, h := newServer(t, nil, Options{Registry: reg, Config: config.Config{Provider: testProvider}})
	rec := do(t, h, s, http.MethodGet, "/api/providers", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("providers = %d %s", rec.Code, rec.Body)
	}
	var payload struct {
		Providers []providerInfo `json:"providers"`
	}
	decode(t, rec, &payload)
	if len(payload.Providers) != 2 || !payload.Providers[0].Available || payload.Providers[0].Name != testProvider || !payload.Providers[0].Default {
		t.Fatalf("providers = %+v", payload.Providers)
	}
	if payload.Providers[1].Available || payload.Providers[1].Name != "off" {
		t.Fatalf("unavailable = %+v", payload.Providers[1])
	}
}

func TestSandboxListWarning(t *testing.T) {
	t.Parallel()

	okProvider := &fakeProvider{name: testProvider, list: []sandbox.Info{{ID: testSandbox, Status: testStatus}}}
	bad := &fakeProvider{name: testOther, listErr: stringError("other is down")}
	reg := registry.New()
	mustRegister(t, reg, bad, okProvider)
	s, h := newServer(t, nil, Options{Registry: reg})
	rec := do(t, h, s, http.MethodGet, "/api/sandboxes", nil)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("other is down")) {
		t.Fatalf("list = %d %s", rec.Code, rec.Body)
	}
	failed := &fakeProvider{name: testProvider, listErr: stringError("lab is down")}
	fs, fh := newServer(t, failed, Options{})
	all := do(t, fh, fs, http.MethodGet, "/api/sandboxes", nil)
	if all.Code != http.StatusInternalServerError {
		t.Fatalf("all failed = %d %s", all.Code, all.Body)
	}
}

func TestOnlyProviderRejectsTheOthers(t *testing.T) {
	t.Parallel()

	lab := &fakeProvider{name: testProvider}
	other := &fakeProvider{name: testOther}
	reg := registry.New()
	mustRegister(t, reg, lab, other)
	s, h := newServer(t, nil, Options{Registry: reg, Provider: testProvider})
	rec := do(t, h, s, http.MethodGet, "/api/providers", nil)
	var payload struct {
		Providers []providerInfo `json:"providers"`
	}
	decode(t, rec, &payload)
	if len(payload.Providers) != 1 || payload.Providers[0].Name != testProvider {
		t.Fatalf("providers = %+v", payload.Providers)
	}
	denied := do(t, h, s, http.MethodPost, "/api/sandboxes", createBody{Name: testSandbox, Provider: testOther})
	if denied.Code != http.StatusBadRequest {
		t.Fatalf("other = %d %s", denied.Code, denied.Body)
	}
}

type stringError string

func (e stringError) Error() string { return string(e) }

func newServer(t *testing.T, p sandbox.Provider, opts Options) (*Server, http.Handler) {
	t.Helper()
	if opts.Registry == nil {
		opts.Registry = registry.New()
		if p != nil {
			mustRegister(t, opts.Registry, p)
		}
	}
	srv, err := New(opts)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	srv.host = "127.0.0.1:9"
	return srv, srv.Handler()
}

func mustRegister(t *testing.T, reg *registry.Registry, providers ...sandbox.Provider) {
	t.Helper()
	for _, p := range providers {
		if err := reg.Register(p); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
}

func do(t *testing.T, h http.Handler, srv *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Host = srv.host
	req.Header.Set(headerSession, srv.sessionHex)
	req.Header.Set(headerCSRF, srv.csrfHex)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func createEvents(t *testing.T, rec *httptest.ResponseRecorder) []createEvent {
	t.Helper()
	var events []createEvent
	dec := json.NewDecoder(bytes.NewReader(rec.Body.Bytes()))
	for dec.More() {
		var ev createEvent
		if err := dec.Decode(&ev); err != nil {
			t.Fatalf("event: %v body %s", err, rec.Body)
		}
		events = append(events, ev)
	}
	if len(events) == 0 {
		t.Fatalf("no events in %q", rec.Body)
	}
	return events
}

func createDone(t *testing.T, rec *httptest.ResponseRecorder) createOut {
	t.Helper()
	events := createEvents(t, rec)
	last := events[len(events)-1]
	if last.Done == nil {
		t.Fatalf("create did not finish: %+v", last)
	}
	return *last.Done
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("json: %v body %s", err, rec.Body)
	}
}
