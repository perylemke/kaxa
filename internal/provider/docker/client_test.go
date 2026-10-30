package docker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const apiToken = "hub-access-token"

// apiServer stands in for the Sandboxes API and the Docker sign-in. It records
// the requests and answers with handler.
type apiServer struct {
	srv  *httptest.Server
	mu   sync.Mutex
	reqs []*http.Request
	body map[string]any
}

func newAPIServer(t *testing.T, handler http.HandlerFunc) (*restClient, *apiServer) {
	t.Helper()
	s := &apiServer{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"` + apiToken + `"}`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.reqs = append(s.reqs, r)
		if r.Body != nil && r.Method != http.MethodGet {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			s.body = body
		}
		s.mu.Unlock()
		handler(w, r)
	})
	s.srv = httptest.NewServer(mux)
	t.Cleanup(s.srv.Close)
	c := newRestClient("me", testPAT, s.srv.Client())
	c.base = s.srv.URL
	c.auth.url = s.srv.URL + "/auth"
	return c, s
}

func (s *apiServer) last(t *testing.T) *http.Request {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.reqs) == 0 {
		t.Fatal("no request")
	}
	return s.reqs[len(s.reqs)-1]
}

const sandboxJSON = `{
 "name":"sandboxes/abc","displayName":"box",
 "core":{"status":"running","imageRef":"ubuntu","createdAt":"2026-01-02T03:04:05Z","etag":"\"v1\"",
  "labels":{"kaxa.managed":"true","kaxa.label.kaxa":"true","kaxa.label.kaxa-sync":"true","other":"x"},
  "endpoint":{"uri":"https://sbx.example.test/abc"}}}`

func TestListReturnsOnlyKaxaSandboxesAcrossPages(t *testing.T) {
	t.Parallel()
	c, srv := newAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageToken") == "" {
			_, _ = w.Write([]byte(`{"sandboxes":[` + sandboxJSON + `,{"name":"sandboxes/mine","displayName":"mine","core":{"status":"running"}}],"nextPageToken":"p2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"sandboxes":[{"name":"sandboxes/z","displayName":"z","core":{"status":"stopped","labels":{"kaxa.managed":"true"}}}]}`))
	})
	got, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != testSandbox || got[1].Name != "z" {
		t.Fatalf("got %+v", got)
	}
	checkParsedRecord(t, got[0])
	if req := srv.last(t); req.Header.Get("Authorization") != "Bearer "+apiToken || req.URL.Query().Get("pageToken") != "p2" {
		t.Fatalf("request = %v %v", req.Header, req.URL)
	}
}

func checkParsedRecord(t *testing.T, r Record) {
	t.Helper()
	if r.Resource != testRes || r.Status != statusRunning || r.Etag != `"v1"` || r.Image != "ubuntu" ||
		r.Endpoint.URI != "https://sbx.example.test/abc" || !slices.Equal(r.Labels, []string{"kaxa", "kaxa-sync"}) ||
		!r.CreatedAt.Equal(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("record = %+v", r)
	}
}

func TestFindFiltersByDisplayName(t *testing.T) {
	t.Parallel()
	c, srv := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"sandboxes":[{"name":"sandboxes/o","displayName":"box","core":{"labels":{}}},` + sandboxJSON + `]}`))
	})
	got, err := c.Find(context.Background(), testSandbox)
	if err != nil || got.Resource != testRes {
		t.Fatalf("got %+v, %v (a sandbox kaxa did not create must be skipped)", got, err)
	}
	if f := srv.last(t).URL.Query().Get("filter"); f != "display_name=box" {
		t.Fatalf("filter = %q", f)
	}
	if _, err := c.Find(context.Background(), "other"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetUsesHeaderEtagAndMapsNotFound(t *testing.T) {
	t.Parallel()
	c, _ := newAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/gone") {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":"notFound","message":"sandbox not found"}`))
			return
		}
		w.Header().Set("ETag", `"v2"`)
		_, _ = w.Write([]byte(sandboxJSON))
	})
	got, err := c.Get(context.Background(), testRes)
	if err != nil || got.Etag != `"v2"` {
		t.Fatalf("got %+v, %v", got, err)
	}
	if _, err := c.Get(context.Background(), "sandboxes/gone"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestCreateSendsLabelsStartupAndIdempotencyKey(t *testing.T) {
	t.Parallel()
	c, srv := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(sandboxJSON))
	})
	rec, err := c.Create(context.Background(), CreateParams{Name: testSandbox, Labels: []string{"kaxa"}, Image: testImage})
	if err != nil || rec.Resource != testRes {
		t.Fatalf("rec = %+v, err = %v", rec, err)
	}
	req := srv.last(t)
	if req.Method != http.MethodPost || req.URL.Path != "/v1/sandboxes" || len(req.Header.Get("Idempotency-Key")) != 32 {
		t.Fatalf("request = %s %s key=%q", req.Method, req.URL.Path, req.Header.Get("Idempotency-Key"))
	}
	body := srv.body
	if body["displayName"] != testSandbox || body["imageRef"] != testImage {
		t.Fatalf("body = %v", body)
	}
	labels, _ := body["labels"].(map[string]any)
	if labels["kaxa.managed"] != "true" || labels["kaxa.label.kaxa"] != "true" {
		t.Fatalf("labels = %v", labels)
	}
	raw, _ := json.Marshal(body)
	for _, want := range []string{`"argv":["tail","-f","/dev/null"]`, `"resources":{"cpus":2,"memoryMib":4096}`, `"onTimeout":"stop"`, `"timeout":"3600s"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("body lacks %s: %s", want, raw)
		}
	}
}

func TestCreateMapsConflict(t *testing.T) {
	t.Parallel()
	c, _ := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"alreadyExists"}`))
	})
	if _, err := c.Create(context.Background(), CreateParams{Name: testSandbox}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("err = %v, want ErrAlreadyExists", err)
	}
}

func TestStartStopSendEtag(t *testing.T) {
	t.Parallel()
	c, srv := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(sandboxJSON))
	})
	rec := Record{Resource: testRes, Etag: `"v7"`}
	for verb, call := range map[string]func(context.Context, Record) error{"start": c.Start, "stop": c.Stop} {
		if err := call(context.Background(), rec); err != nil {
			t.Fatal(err)
		}
		req := srv.last(t)
		if req.Method != http.MethodPost || req.URL.Path != "/v1/sandboxes/abc/"+verb || req.Header.Get("If-Match") != `"v7"` {
			t.Fatalf("%s: %s %s If-Match=%q", verb, req.Method, req.URL.Path, req.Header.Get("If-Match"))
		}
	}
}

func TestDeleteForcesAndMapsNotFound(t *testing.T) {
	t.Parallel()
	status := http.StatusAccepted
	c, srv := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		if status == http.StatusNotFound {
			_, _ = w.Write([]byte(`{"code":"notFound"}`))
		}
	})
	rec := Record{Resource: testRes, Etag: `"v1"`}
	if err := c.Delete(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	req := srv.last(t)
	if req.Method != http.MethodDelete || req.URL.Query().Get("force") != "true" || req.Header.Get("If-Match") != `"v1"` {
		t.Fatalf("request = %s %s", req.Method, req.URL)
	}
	status = http.StatusNotFound
	if err := c.Delete(context.Background(), rec); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRenewToleratesUnimplemented(t *testing.T) {
	t.Parallel()
	status, body := http.StatusOK, `{"code":"unimplemented"}`
	c, srv := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	rec := Record{Resource: testRes}
	if err := c.Renew(context.Background(), rec, time.Hour); err != nil {
		t.Fatal(err)
	}
	if srv.body["timeout"] != "3600s" {
		t.Fatalf("body = %v", srv.body)
	}
	status = http.StatusNotImplemented
	if err := c.Renew(context.Background(), rec, time.Hour); err != nil {
		t.Fatalf("unimplemented must not fail: %v", err)
	}
	status, body = http.StatusForbidden, `{"code":"permissionDenied"}`
	if err := c.Renew(context.Background(), rec, time.Hour); err == nil {
		t.Fatal("other failures must be reported")
	}
}

func TestCredentialRequestsExecPermission(t *testing.T) {
	t.Parallel()
	c, srv := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"token":"endpoint-tok","expireTime":"2026-01-01T00:00:00Z"}`))
	})
	cred, err := c.Credential(context.Background(), Record{Resource: testRes})
	if err != nil || cred.Token != "endpoint-tok" {
		t.Fatalf("cred = %+v, err = %v", cred, err)
	}
	if req := srv.last(t); req.URL.Path != "/v1/sandboxes/abc/endpoint-credentials" {
		t.Fatalf("path = %s", req.URL.Path)
	}
	raw, _ := json.Marshal(srv.body)
	if string(raw) != `{"permissions":["sandboxesExec"],"ttl":"300s"}` {
		t.Fatalf("body = %s", raw)
	}
}

const (
	testPortURL = "https://8000-abc.sbx.example.test"
	portsPath   = "/v1/sandboxes/abc/ports"
)

func TestPublishPortSendsPortAndReturnsURL(t *testing.T) {
	t.Parallel()
	c, srv := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"number":8000,"protocol":"tcp","url":"` + testPortURL + `"}`))
	})
	got, err := c.PublishPort(context.Background(), Record{Resource: testRes}, 8000)
	if err != nil || got != testPortURL {
		t.Fatalf("url = %q, err = %v", got, err)
	}
	req := srv.last(t)
	if req.Method != http.MethodPost || req.URL.Path != portsPath || len(req.Header.Get("Idempotency-Key")) != 32 {
		t.Fatalf("request = %s %s key=%q", req.Method, req.URL.Path, req.Header.Get("Idempotency-Key"))
	}
	raw, _ := json.Marshal(srv.body)
	if string(raw) != `{"port":{"number":8000}}` {
		t.Fatalf("body = %s", raw)
	}
}

func TestPublishPortWithoutURL(t *testing.T) {
	t.Parallel()
	c, _ := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"number":8000}`))
	})
	if _, err := c.PublishPort(context.Background(), Record{Resource: testRes}, 8000); err == nil {
		t.Fatal("a port without a URL must be an error")
	}
}

func TestPublishPortAlreadyPublishedReturnsTheURL(t *testing.T) {
	t.Parallel()
	c, srv := newAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"code":"alreadyExists"}`))
			return
		}
		_, _ = w.Write([]byte(`{"name":"sandboxes/abc","displayName":"box","core":{"status":"running","ports":[` +
			`{"sandboxPort":9000,"url":"https://other.example.test"},{"sandboxPort":8000,"url":"` + testPortURL + `"}]}}`))
	})
	got, err := c.PublishPort(context.Background(), Record{Resource: testRes}, 8000)
	if err != nil || got != testPortURL {
		t.Fatalf("url = %q, err = %v", got, err)
	}
	if req := srv.last(t); req.Method != http.MethodGet || req.URL.Path != "/v1/sandboxes/abc" {
		t.Fatalf("last request = %s %s", req.Method, req.URL.Path)
	}
}

func TestPublishPortKeepsOtherErrors(t *testing.T) {
	t.Parallel()
	c, _ := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":"permissionDenied"}`))
	})
	_, err := c.PublishPort(context.Background(), Record{Resource: testRes}, 8000)
	if apiErr, ok := errors.AsType[*APIError](err); !ok || apiErr.Code != codePermissionDenied {
		t.Fatalf("err = %#v", err)
	}
}

func TestRecordParsesPublishedPorts(t *testing.T) {
	t.Parallel()
	var w wireSandbox
	raw := `{"name":"sandboxes/abc","displayName":"box","core":{"ports":[{"sandboxPort":8000,"url":"` + testPortURL + `"}]}}`
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		t.Fatal(err)
	}
	got := w.record("").Ports
	if len(got) != 1 || got[0] != (PublishedPort{Port: 8000, URL: testPortURL}) {
		t.Fatalf("ports = %+v", got)
	}
}

func TestCredentialWithoutToken(t *testing.T) {
	t.Parallel()
	c, _ := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) })
	if _, err := c.Credential(context.Background(), Record{Resource: testRes}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestAPIErrorsCarryCodeAndMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status int
		body   string
		want   APIError
	}{
		{"structured", 429, `{"code":"resourceExhausted","message":"too many"}`, APIError{Status: 429, Code: "resourceExhausted", Message: "too many"}},
		{"html", 502, `<html>`, APIError{Status: 502}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, _ := newAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})
			_, err := c.Get(context.Background(), testRes)
			if apiErr, ok := errors.AsType[*APIError](err); !ok || *apiErr != tt.want {
				t.Fatalf("err = %#v, want %+v", err, tt.want)
			}
		})
	}
}

func TestRequestErrorDoesNotRepeatURL(t *testing.T) {
	t.Parallel()
	c, srv := newAPIServer(t, func(http.ResponseWriter, *http.Request) {})
	srv.srv.Close()
	_, err := c.Get(context.Background(), testRes)
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), srv.srv.URL) {
		t.Fatalf("err = %v, must not repeat the URL", err)
	}
}

func TestNewCreateBodyKeepsLabelsApart(t *testing.T) {
	t.Parallel()
	b := newCreateBody(CreateParams{Name: testSandbox, Labels: []string{"a"}, Image: "img"})
	if len(b.Labels) != 2 || b.Labels[managedLabel] != labelTrue || b.Labels[labelPrefix+"a"] != labelTrue {
		t.Fatalf("labels = %v", b.Labels)
	}
}
