package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	maxBody         = 1 << 20
	contentJSON     = "application/json; charset=utf-8"
	contentHTML     = "text/html; charset=utf-8"
	contentCSS      = "text/css; charset=utf-8"
	contentJS       = "text/javascript; charset=utf-8"
	headerCSP       = "Content-Security-Policy"
	headerNosniff   = "X-Content-Type-Options"
	headerCache     = "Cache-Control"
	cspValue        = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"
	cacheNoStore    = "no-store"
	contentTypeOpts = "nosniff"
)

type errorBody struct {
	Error string `json:"error"`
	Hint  string `json:"hint,omitempty"`
}

func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set(headerCSP, cspValue)
	w.Header().Set(headerNosniff, contentTypeOpts)
	w.Header().Set(headerCache, cacheNoStore)
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", contentJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) writeStatus(w http.ResponseWriter, status int, message, hint string) {
	s.writeJSON(w, status, errorBody{Error: s.redact(message), Hint: s.redact(hint)})
}

func (s *Server) writeErr(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := err.Error()
	hint := ""
	if ue, ok := usererr.As(err); ok {
		status = http.StatusBadRequest
		message = ue.Message
		hint = ue.Hint
	}
	if notFound(message) {
		status = http.StatusNotFound
	}
	if errors.Is(err, sandbox.ErrUnsupported) {
		status = http.StatusBadRequest
	}
	message = s.redact(message)
	hint = s.redact(hint)
	if status == http.StatusInternalServerError {
		s.log(message)
	}
	s.writeJSON(w, status, errorBody{Error: message, Hint: hint})
}

func notFound(message string) bool {
	return strings.Contains(strings.ToLower(message), "not found")
}

func decodeJSON(r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return usererr.New("invalid request", "send a JSON object")
	}
	var extra struct{}
	if err := dec.Decode(&extra); err != io.EOF {
		return usererr.New("invalid request", "send one JSON object")
	}
	return nil
}
