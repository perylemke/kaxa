package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
)

var fixedNow = time.Date(2026, 11, 27, 9, 30, 0, 0, time.Local)

func loggedServer(t *testing.T, p sandbox.Provider, opts Options) (*Server, http.Handler, *bytes.Buffer) {
	t.Helper()
	stderr := &bytes.Buffer{}
	opts.Stderr = stderr
	s, h := newServer(t, p, opts)
	s.now = func() time.Time { return fixedNow }
	return s, h, stderr
}

func TestCreateStreamsStepsAndLogsThem(t *testing.T) {
	t.Parallel()

	s, h, stderr := loggedServer(t, &fakeProvider{name: testProvider}, Options{})
	rec := do(t, h, s, http.MethodPost, "/api/sandboxes", createBody{Name: testSandbox, Provider: testProvider})
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != contentNDJSON {
		t.Fatalf("create = %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	events := createEvents(t, rec)
	if len(events) != 2 || events[0].Step != "creating box on lab…" || events[1].Done == nil || events[1].Done.ID != testSandbox {
		t.Fatalf("events = %+v", events)
	}
	log := stderr.String()
	for _, want := range []string{
		"09:30:00  info   create    lab/box                   started\n",
		"09:30:00  info   create    lab/box                   creating box on lab…\n",
		"09:30:00  info   create    lab/box                   done in 0s\n",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("log is missing %q\n%s", want, log)
		}
	}
}

func TestCreateFailureIsAnEventAndAnErrorLine(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testProvider, createErr: stringError("backend is down")}
	s, h, stderr := loggedServer(t, fp, Options{})
	rec := do(t, h, s, http.MethodPost, "/api/sandboxes", createBody{Name: testSandbox, Provider: testProvider})
	events := createEvents(t, rec)
	last := events[len(events)-1]
	if last.Done != nil || last.Error != "backend is down" || last.ID != "" {
		t.Fatalf("last = %+v", last)
	}
	if !strings.Contains(stderr.String(), "error  create    lab/box                   backend is down\n") {
		t.Fatalf("log = %s", stderr)
	}
}

func TestRefusedCreateIsLoggedWithItsTarget(t *testing.T) {
	t.Parallel()

	s, h, stderr := loggedServer(t, &fakeProvider{name: testProvider}, Options{})
	rec := do(t, h, s, http.MethodPost, "/api/sandboxes", createBody{Name: testSandbox, Provider: testProvider, Running: true})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	if got := stderr.String(); !strings.Contains(got, "error  create    lab/box") {
		t.Fatalf("log = %s", got)
	}
}

func TestActionsAreLoggedOnSuccessAndFailure(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testProvider, upgradeErr: stringError("upgrade refused by lab")}
	s, h, stderr := loggedServer(t, capable{fp}, Options{})
	if rec := do(t, h, s, http.MethodPost, "/api/sandboxes/lab/box/restart", map[string]string{}); rec.Code != http.StatusOK {
		t.Fatalf("restart = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, s, http.MethodPost, "/api/sandboxes/lab/box/upgrade", map[string]string{}); rec.Code != http.StatusInternalServerError {
		t.Fatalf("upgrade = %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, s, http.MethodPost, "/api/sandboxes/lab/box/destroy", destroyBody{Confirm: testSandbox}); rec.Code != http.StatusOK {
		t.Fatalf("destroy = %d %s", rec.Code, rec.Body)
	}
	log := stderr.String()
	for _, want := range []string{
		"info   restart   lab/box                   done in 0s\n",
		"error  upgrade   lab/box                   upgrade refused by lab\n",
		"info   destroy   lab/box                   done in 0s\n",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("log is missing %q\n%s", want, log)
		}
	}
}

func TestReadsStayQuietUnlessTheyFail(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testProvider, list: []sandbox.Info{{ID: testSandbox, Status: testStatus}}}
	s, h, stderr := loggedServer(t, fp, Options{})
	for _, path := range []string{"/api/providers", "/api/sandboxes", "/api/sandboxes/lab/box"} {
		if rec := do(t, h, s, http.MethodGet, path, nil); rec.Code != http.StatusOK {
			t.Fatalf("%s = %d %s", path, rec.Code, rec.Body)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("reads were logged: %s", stderr)
	}

	fp.inspectErr = stringError("sandbox box not found")
	if rec := do(t, h, s, http.MethodGet, "/api/sandboxes/lab/box", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("inspect = %d", rec.Code)
	}
	if !strings.Contains(stderr.String(), "error  inspect   lab/box                   sandbox box not found\n") {
		t.Fatalf("log = %s", stderr)
	}
}

func TestListWarningsAreLogged(t *testing.T) {
	t.Parallel()

	good := &fakeProvider{name: testProvider, list: []sandbox.Info{{ID: testSandbox, Status: testStatus}}}
	bad := &fakeProvider{name: testOther, listErr: stringError("other is down")}
	reg := registry.New()
	mustRegister(t, reg, bad, good)
	s, h, stderr := loggedServer(t, nil, Options{Registry: reg})
	if rec := do(t, h, s, http.MethodGet, "/api/sandboxes", nil); rec.Code != http.StatusOK {
		t.Fatalf("list = %d %s", rec.Code, rec.Body)
	}
	if got := stderr.String(); !strings.Contains(got, "warn   list") || !strings.Contains(got, "other is down") {
		t.Fatalf("log = %s", got)
	}
}

func TestRejectedRequestsAreLoggedWithoutTheirHost(t *testing.T) {
	t.Parallel()

	s, h, stderr := loggedServer(t, &fakeProvider{name: testProvider}, Options{})
	req := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
	req.Host = "evil.example"
	h.ServeHTTP(httptest.NewRecorder(), req)
	req = httptest.NewRequest(http.MethodGet, "/api/providers", nil)
	req.Host = s.host
	h.ServeHTTP(httptest.NewRecorder(), req)
	req = httptest.NewRequest(http.MethodPost, "/api/sandboxes", nil)
	req.Host = s.host
	req.Header.Set(headerSession, s.sessionHex)
	h.ServeHTTP(httptest.NewRecorder(), req)

	log := stderr.String()
	for _, want := range []string{"warn   request", "rejected host", "missing session", "missing csrf token"} {
		if !strings.Contains(log, want) {
			t.Fatalf("log is missing %q\n%s", want, log)
		}
	}
	if strings.Contains(log, "evil.example") || strings.Contains(log, s.sessionHex) {
		t.Fatalf("log repeats caller input\n%s", log)
	}
}

func TestEventKeepsEveryLineOnOneLine(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	s := &Server{stderr: &out, secrets: []string{"s3cret"}, now: func() time.Time { return fixedNow }}
	s.event(levelError, "create", "lab/box", "first\nsecond s3cret\r\nthird")
	s.event(levelInfo, "ui", "", "")
	want := "09:30:00  error  create    lab/box                   first second [redacted] third\n"
	if out.String() != want {
		t.Fatalf("log = %q, want %q", out.String(), want)
	}

	quiet := &Server{}
	quiet.event(levelInfo, "ui", "", "nobody listens")
}

func TestHelpersAroundTheLog(t *testing.T) {
	t.Parallel()

	if got := elapsed(-time.Second); got != "0s" {
		t.Fatalf("negative = %q", got)
	}
	if got := elapsed(35149 * time.Millisecond); got != "35.1s" {
		t.Fatalf("elapsed = %q", got)
	}
	s := &Server{}
	if s.clock().IsZero() {
		t.Fatal("default clock is zero")
	}

	plain := httptest.NewRecorder()
	noteDetail(plain, "ignored")
	noteTarget(plain, "ignored")

	rec := &recorder{ResponseWriter: httptest.NewRecorder()}
	if _, err := rec.Write([]byte("x")); err != nil || rec.status != http.StatusOK {
		t.Fatalf("write = %v status %d", err, rec.status)
	}
	rec.WriteHeader(http.StatusTeapot)
	if rec.status != http.StatusOK || rec.Unwrap() == nil {
		t.Fatalf("status = %d", rec.status)
	}
}

func TestAuditIgnoresUnknownRoutes(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	s := &Server{stderr: &out}
	req := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
	s.audit(req, &recorder{status: http.StatusInternalServerError}, time.Now())
	if out.Len() != 0 {
		t.Fatalf("log = %s", out.String())
	}

	req.Pattern = "GET /api/providers"
	s.audit(req, &recorder{status: http.StatusBadGateway}, time.Now())
	if !strings.Contains(out.String(), "Bad Gateway") {
		t.Fatalf("log = %s", out.String())
	}
}
