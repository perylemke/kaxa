package web

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/config"
	"github.com/perylemke/kaxa/internal/registry"
)

func TestSecurityRejectsHostSessionAndCSRF(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testProvider}
	s, h := newServer(t, fp, Options{})

	tests := []struct {
		name   string
		host   string
		header map[string]string
		method string
		path   string
		want   int
	}{
		{name: "wrong host", host: "evil.example", method: http.MethodGet, path: "/", want: http.StatusForbidden},
		{name: "missing session", host: s.host, method: http.MethodGet, path: "/api/providers", want: http.StatusUnauthorized},
		{
			name: "missing csrf", host: s.host, method: http.MethodPost, path: "/api/sandboxes",
			header: map[string]string{headerSession: s.sessionHex}, want: http.StatusForbidden,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(`{"name":"box","provider":"lab"}`))
			req.Host = tt.host
			for key, value := range tt.header {
				req.Header.Set(key, value)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d %s", rec.Code, rec.Body)
			}
			if strings.Contains(rec.Body.String(), s.sessionHex) {
				t.Fatalf("response leaked the session: %s", rec.Body)
			}
		})
	}
}

func TestResponsesHideProviderTokens(t *testing.T) {
	t.Parallel()

	const secret = "kaxa-provider-token-do-not-leak"
	fp := &fakeProvider{name: testProvider, createErr: stringError("backend failed " + secret)}
	stderr := &bytes.Buffer{}
	s, h := newServer(t, fp, Options{
		Config: config.Config{Sprites: config.SpritesConfig{Token: secret}},
		Stderr: stderr,
	})

	page := do(t, h, s, http.MethodGet, "/", nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), s.sessionHex) {
		t.Fatalf("page = %d", page.Code)
	}
	if strings.Contains(page.Body.String(), secret) {
		t.Fatal("page contains the provider token")
	}

	rec := do(t, h, s, http.MethodPost, "/api/sandboxes", createBody{Name: testSandbox, Provider: testProvider})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), secret) || strings.Contains(stderr.String(), secret) {
		t.Fatalf("token leaked\nbody %s\nlog %s", rec.Body, stderr)
	}
	if !strings.Contains(rec.Body.String(), "[redacted]") || !strings.Contains(stderr.String(), "[redacted]") {
		t.Fatalf("token was not redacted\nbody %s\nlog %s", rec.Body, stderr)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("cors header is set")
	}
	if rec.Header().Get(headerCache) != cacheNoStore || rec.Header().Get(headerNosniff) != contentTypeOpts {
		t.Fatalf("headers = %v", rec.Header())
	}
	if !strings.Contains(rec.Header().Get(headerCSP), "default-src 'self'") {
		t.Fatalf("csp = %s", rec.Header().Get(headerCSP))
	}
}

func TestStaticAssetsAndMissingFile(t *testing.T) {
	t.Parallel()

	s, h := newServer(t, &fakeProvider{name: testProvider}, Options{})
	css := do(t, h, s, http.MethodGet, "/app.css", nil)
	js := do(t, h, s, http.MethodGet, "/app.js", nil)
	if css.Code != http.StatusOK || !strings.Contains(css.Body.String(), "#07111f") {
		t.Fatalf("css = %d %s", css.Code, css.Body)
	}
	if js.Code != http.StatusOK || !bytes.Contains(js.Body.Bytes(), []byte(headerSession)) || !bytes.Contains(js.Body.Bytes(), []byte(headerCSRF)) {
		t.Fatalf("js missing session headers")
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.Host = s.host
	s.serveAsset(rec, req, "assets/missing.txt", contentCSS)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing = %d", rec.Code)
	}
}

func TestLoopbackHostRejectsOtherAddresses(t *testing.T) {
	t.Parallel()

	if _, err := loopbackHost(&net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 1}); err == nil {
		t.Fatal("accepted a non-loopback address")
	}
	if _, err := loopbackHost(&net.UnixAddr{Name: "x", Net: "unix"}); err == nil {
		t.Fatal("accepted a non-tcp address")
	}
	got, err := loopbackHost(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4000})
	if err != nil || got != "127.0.0.1:4000" {
		t.Fatalf("host = %q %v", got, err)
	}
}

func TestNewRejectsNilRegistry(t *testing.T) {
	t.Parallel()

	if _, err := New(Options{}); err == nil {
		t.Fatal("nil registry was accepted")
	}
	reg := registry.New()
	if _, err := New(Options{Registry: reg, Port: 70000}); err == nil {
		t.Fatal("port 70000 was accepted")
	}
}
