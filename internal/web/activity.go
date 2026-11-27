package web

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Levels written to the terminal log.
const (
	levelInfo  = "info"
	levelWarn  = "warn"
	levelError = "error"
)

// action names a route in the terminal log. quiet routes log only when they fail,
// so refreshing the list does not fill the terminal.
type action struct {
	name  string
	quiet bool
}

var actions = map[string]action{
	"GET /api/providers":                             {name: "providers", quiet: true},
	"GET /api/dirs":                                  {name: "dirs", quiet: true},
	"GET /api/sandboxes":                             {name: "list", quiet: true},
	"POST /api/sandboxes":                            {name: "create", quiet: true},
	"GET /api/sandboxes/{provider}/{name}":           {name: "inspect", quiet: true},
	"POST /api/sandboxes/{provider}/{name}/restart":  {name: "restart"},
	"POST /api/sandboxes/{provider}/{name}/upgrade":  {name: "upgrade"},
	"POST /api/sandboxes/{provider}/{name}/url-auth": {name: "url-auth"},
	"POST /api/sandboxes/{provider}/{name}/destroy":  {name: "destroy"},
}

// recorder remembers what a handler answered so the log can say what happened.
type recorder struct {
	http.ResponseWriter
	status int
	detail string
	target string
}

func (r *recorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// Unwrap lets http.NewResponseController reach the real writer to flush.
func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func noteDetail(w http.ResponseWriter, detail string) {
	if rec, ok := w.(*recorder); ok {
		rec.detail = detail
	}
}

func noteTarget(w http.ResponseWriter, target string) {
	if rec, ok := w.(*recorder); ok {
		rec.target = target
	}
}

func (s *Server) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// event writes one line of the terminal log: time, level, action, provider/sandbox, detail.
// The line never carries a provider token, the session, or the CSRF token.
func (s *Server) event(level, name, target, detail string) {
	detail = oneLine(detail)
	if s.stderr == nil || detail == "" {
		return
	}
	if target = oneLine(target); target == "" {
		target = "-"
	}
	_, _ = fmt.Fprintf(
		s.stderr, "%s  %-5s  %-8s  %-24s  %s\n",
		s.clock().Format("15:04:05"), level, name, s.redact(target), s.redact(detail),
	)
}

func (s *Server) log(line string) {
	s.event(levelInfo, "ui", "", line)
}

// audit logs the outcome of an API request after its handler ran.
func (s *Server) audit(r *http.Request, rec *recorder, started time.Time) {
	act, known := actions[r.Pattern]
	if !known {
		return
	}
	target := rec.target
	if provider, name := r.PathValue("provider"), r.PathValue("name"); provider != "" && name != "" {
		target = provider + "/" + name
	}
	if rec.status >= http.StatusBadRequest {
		detail := rec.detail
		if detail == "" {
			detail = http.StatusText(rec.status)
		}
		s.event(levelError, act.name, target, detail)
		return
	}
	if !act.quiet {
		s.event(levelInfo, act.name, target, "done in "+elapsed(s.clock().Sub(started)))
	}
}

func elapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return d.Round(100 * time.Millisecond).String()
}

// oneLine keeps a log entry on one line, whatever a provider put in its message.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
