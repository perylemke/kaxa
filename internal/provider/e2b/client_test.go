package e2b

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	testKey     = "e2b_secret_key"
	testSandbox = "sbx-1"
	testWeb     = "web"
)

func newTestREST(t *testing.T, h http.HandlerFunc) *restClient {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := newRESTClient(testKey)
	c.base = srv.URL
	c.http = srv.Client()
	return c
}

func listedJSON(id, name, state string, extra map[string]string) map[string]any {
	meta := map[string]string{metaManaged: metaTrue, metaName: name, metaLabels: "kaxa,sync"}
	maps.Copy(meta, extra)
	return map[string]any{
		"sandboxID": id, "state": state, "startedAt": "2026-10-29T10:00:00Z", "metadata": meta,
	}
}

func TestRESTCreateSendsSecureDefaults(t *testing.T) {
	t.Parallel()

	var got map[string]any
	var key, path string
	c := newTestREST(t, func(w http.ResponseWriter, r *http.Request) {
		key, path = r.Header.Get("X-API-Key"), r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"sandboxID":"sbx-1","envdAccessToken":"tok"}`))
	})

	id, err := c.Create(context.Background(), CreateParams{Name: testWeb, Labels: []string{"kaxa", "sync"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id != testSandbox || key != testKey || path != "/v2/sandboxes" {
		t.Fatalf("id = %q key = %q path = %q", id, key, path)
	}
	if got["templateID"] != defaultTemplate || got["autoPause"] != true || got["timeout"] != float64(sandboxTimeout) {
		t.Fatalf("body = %v", got)
	}
	network, _ := got["network"].(map[string]any)
	if network["allowPublicTraffic"] != false {
		t.Fatalf("network = %v, want a private sandbox", network)
	}
	meta, _ := got["metadata"].(map[string]any)
	if meta[metaName] != testWeb || meta[metaManaged] != metaTrue || meta[metaLabels] != "kaxa,sync" {
		t.Fatalf("metadata = %v", meta)
	}
	if _, ok := meta[metaAPIPort]; ok {
		t.Fatalf("metadata has an api port without an API: %v", meta)
	}
}

func TestRESTCreateTemplate(t *testing.T) {
	t.Parallel()

	var got map[string]any
	c := newTestREST(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"sandboxID":"sbx-1"}`))
	})

	if _, err := c.Create(context.Background(), CreateParams{Name: testWeb, Template: "team/big:v2"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	meta, _ := got["metadata"].(map[string]any)
	if got["templateID"] != "team/big:v2" || meta[metaTemplate] != "team/big:v2" {
		t.Fatalf("templateID = %v meta = %v", got["templateID"], meta)
	}
}

func TestRESTCreateUnknownTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template string
		status   int
		want     error
	}{
		{name: "custom template not found", template: "nope", status: http.StatusNotFound, want: ErrTemplateNotFound},
		{name: "other error with a template", template: "big", status: http.StatusInternalServerError},
		{name: "not found without a template", status: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := newTestREST(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"code":1,"message":"boom"}`))
			})
			_, err := c.Create(context.Background(), CreateParams{Name: testWeb, Template: tt.template})
			if err == nil || errors.Is(err, ErrTemplateNotFound) != (tt.want != nil) {
				t.Fatalf("err = %v, want template-not-found = %v", err, tt.want != nil)
			}
		})
	}
}

func TestRESTCreatePublishedAPI(t *testing.T) {
	t.Parallel()

	var got map[string]any
	c := newTestREST(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"sandboxID":"sbx-1"}`))
	})

	if _, err := c.Create(context.Background(), CreateParams{Name: "api", APIPort: 8000, PublicTraffic: true}); err != nil {
		t.Fatalf("create: %v", err)
	}
	network, _ := got["network"].(map[string]any)
	meta, _ := got["metadata"].(map[string]any)
	if network["allowPublicTraffic"] != true || meta[metaAPIPort] != "8000" {
		t.Fatalf("network = %v meta = %v", network, meta)
	}
	if _, ok := meta[metaTemplate]; ok {
		t.Fatalf("metadata names a template for the base template: %v", meta)
	}
}

func TestRESTListFiltersAndPages(t *testing.T) {
	t.Parallel()

	var queries []string
	c := newTestREST(t, func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		if r.URL.Query().Get("nextToken") == "" {
			w.Header().Set("X-Next-Token", "page2")
			_ = json.NewEncoder(w).Encode([]any{
				listedJSON("sbx-a", "a", stateRunning, map[string]string{metaAPIPort: "8000"}),
				map[string]any{"sandboxID": "other", "state": stateRunning, "metadata": map[string]string{"app": "x"}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode([]any{listedJSON("sbx-b", "b", statePaused, nil)})
	})

	got, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 || got[0].Name != "a" || got[1].Name != "b" {
		t.Fatalf("records = %+v", got)
	}
	if got[0].APIPort != 8000 || got[0].Labels[1] != "sync" || got[1].State != statePaused {
		t.Fatalf("records = %+v", got)
	}
	if len(queries) != 2 || !strings.Contains(queries[0], "kaxa.managed%3Dtrue") || !strings.Contains(queries[0], "state=running%2Cpaused") {
		t.Fatalf("queries = %v", queries)
	}
}

func TestRESTFind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		page    []any
		want    string
		wantErr error
	}{
		{name: "found", page: []any{listedJSON("sbx-1", testWeb, stateRunning, nil)}, want: "sbx-1"},
		{name: "empty", page: []any{}, wantErr: ErrNotFound},
		{name: "other name ignored", page: []any{listedJSON("sbx-2", "api", stateRunning, nil)}, wantErr: ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var query string
			c := newTestREST(t, func(w http.ResponseWriter, r *http.Request) {
				query = r.URL.Query().Get("metadata")
				_ = json.NewEncoder(w).Encode(tt.page)
			})
			rec, err := c.Find(context.Background(), testWeb)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if rec.ID != tt.want {
				t.Fatalf("id = %q, want %q", rec.ID, tt.want)
			}
			if query != "kaxa.managed=true&kaxa.name=web" {
				t.Fatalf("metadata filter = %q", query)
			}
		})
	}
}

func TestRESTKillAndPause(t *testing.T) {
	t.Parallel()

	var calls []string
	var pause map[string]bool
	c := newTestREST(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/pause") {
			_ = json.NewDecoder(r.Body).Decode(&pause)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.Kill(context.Background(), testSandbox); err != nil {
		t.Fatalf("kill: %v", err)
	}
	if err := c.Pause(context.Background(), testSandbox, false); err != nil {
		t.Fatalf("pause: %v", err)
	}
	want := []string{"DELETE /sandboxes/sbx-1", "POST /sandboxes/sbx-1/pause"}
	if strings.Join(calls, ";") != strings.Join(want, ";") || pause["memory"] != false {
		t.Fatalf("calls = %v pause = %v", calls, pause)
	}
}

func TestRESTNotFound(t *testing.T) {
	t.Parallel()

	c := newTestREST(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":404,"message":"sandbox not found"}`))
	})
	ctx := context.Background()
	if err := c.Kill(ctx, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("kill err = %v", err)
	}
	if err := c.Pause(ctx, "x", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("pause err = %v", err)
	}
	if _, err := c.Connect(ctx, "x", ConnectParams{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("connect err = %v", err)
	}
}

func TestRESTConnect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		params     ConnectParams
		domain     string
		wantMemory bool
		wantDomain string
	}{
		{name: "plain", wantDomain: defaultDomain},
		{name: "reboot", params: ConnectParams{Reboot: true}, wantMemory: true, wantDomain: defaultDomain},
		{name: "domain from API", domain: `"domain":"edge.example.com",`, wantDomain: "edge.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var body map[string]any
			c := newTestREST(t, func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&body)
				_, _ = w.Write([]byte(`{` + tt.domain + `"sandboxID":"sbx-1","envdAccessToken":"daemon-token"}`))
			})
			conn, err := c.Connect(context.Background(), testSandbox, tt.params)
			if err != nil {
				t.Fatalf("connect: %v", err)
			}
			if conn.AccessToken != "daemon-token" || conn.ID != testSandbox || conn.Domain != tt.wantDomain {
				t.Fatalf("conn = %+v", conn)
			}
			_, hasMemory := body["memory"]
			if hasMemory != tt.wantMemory || body["timeout"] != float64(sandboxTimeout) {
				t.Fatalf("body = %v", body)
			}
		})
	}
}

func TestRESTConnectWithoutTokenFails(t *testing.T) {
	t.Parallel()

	c := newTestREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"sandboxID":"sbx-1"}`))
	})
	if _, err := c.Connect(context.Background(), testSandbox, ConnectParams{}); err == nil {
		t.Fatal("connect without an access token accepted")
	}
}

func TestRESTErrorsCarryServerMessage(t *testing.T) {
	t.Parallel()

	c := newTestREST(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":401,"message":"invalid api key"}`))
	})
	_, err := c.List(context.Background())
	api, ok := errors.AsType[*APIError](err)
	if !ok || api.Status != http.StatusUnauthorized || api.Message != "invalid api key" {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), testKey) {
		t.Fatalf("error leaks the key: %v", err)
	}
}

func TestRESTListContextCanceled(t *testing.T) {
	t.Parallel()

	c := newTestREST(t, func(http.ResponseWriter, *http.Request) {})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.List(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}
