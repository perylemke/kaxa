package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

var pngSignature = []byte("\x89PNG\r\n\x1a\n")

func TestLogoAndFaviconArePNGs(t *testing.T) {
	t.Parallel()

	s, h := newServer(t, nil, Options{})
	for _, path := range []string{"/kaxa-logo.png", "/favicon.png"} {
		t.Run(path, func(t *testing.T) {
			rec := do(t, h, s, http.MethodGet, path, nil)
			if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != contentPNG {
				t.Fatalf("status = %d type = %q", rec.Code, rec.Header().Get("Content-Type"))
			}
			if !bytes.HasPrefix(rec.Body.Bytes(), pngSignature) {
				t.Fatalf("%s is not a PNG", path)
			}
		})
	}
}

func TestLogoRefusesAForeignHost(t *testing.T) {
	t.Parallel()

	_, h := newServer(t, nil, Options{})
	req := httptest.NewRequest(http.MethodGet, "/kaxa-logo.png", nil)
	req.Host = "evil.example"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("a foreign Host must be refused, got %d", rec.Code)
	}
}
