package vercel

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func createBox(t *testing.T, p *Provider, mutate ...func(*sandbox.Spec)) sandbox.ID {
	t.Helper()
	spec := sandbox.Spec{Name: boxName, Labels: []string{wrapperArg0}}
	for _, m := range mutate {
		m(&spec)
	}
	id, err := p.Create(context.Background(), spec)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return id
}

func TestProviderName(t *testing.T) {
	t.Parallel()
	if got := New().Name(); got != ProviderName {
		t.Fatalf("Name() = %q", got)
	}
}

func TestConfigure(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ name, token, project string }{
		{"no token", "", "proj"},
		{"no project", "tok", ""},
		{"nothing", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := New()
			if err := p.Configure(tt.token, tt.project, ""); !errors.Is(err, sandbox.ErrNotConfigured) {
				t.Fatalf("Configure() error = %v, want ErrNotConfigured", err)
			}
			if p.Available() {
				t.Fatal("Available() = true without credentials")
			}
			if _, err := p.List(context.Background()); !errors.Is(err, sandbox.ErrNotConfigured) {
				t.Fatalf("List() error = %v", err)
			}
		})
	}
	t.Run("keeps and replaces the client", func(t *testing.T) {
		t.Parallel()
		p := New()
		if err := p.Configure("tok", "proj", "team_1"); err != nil {
			t.Fatal(err)
		}
		first := p.api
		if err := p.Configure("tok", "proj", "team_1"); err != nil || p.api != first {
			t.Fatalf("same config replaced the client (err = %v)", err)
		}
		if err := p.Configure("tok", "other", "team_1"); err != nil || p.api == first {
			t.Fatalf("new project kept the client (err = %v)", err)
		}
		if !p.Available() {
			t.Fatal("Available() = false")
		}
	})
}

func TestNewWithBaseURL(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		base string
		ok   bool
	}{
		{"https://api.example.com", true},
		{"http://127.0.0.1:1234", true},
		{"http://[::1]:1234", true},
		{"http://localhost:1234", true},
		{"http://example.com", false},
		{"://bad", false},
		{"no-host", false},
	} {
		t.Run(tt.base, func(t *testing.T) {
			t.Parallel()
			_, err := NewWithBaseURL(tt.base, "tok", "proj", "")
			if (err == nil) != tt.ok {
				t.Fatalf("error = %v, want ok = %v", err, tt.ok)
			}
		})
	}
}

//nolint:gocyclo // one scenario that checks many fields of a single response
func TestCapabilities(t *testing.T) {
	t.Parallel()
	p := New()
	p.SupportsTemplate()
	p.SupportsCPU()
	p.SupportsNetworkBlock()
	caps := sandbox.CapabilitiesOf(p)
	if !caps.Sync || !caps.Setup || !caps.API || !caps.Layout || !caps.Restart || !caps.Template || !caps.CPU || !caps.NetworkBlock {
		t.Fatalf("capabilities = %+v", caps)
	}
	if !caps.Snapshot || caps.SnapshotRestore != string(sandbox.RestoreInPlace) {
		t.Fatalf("snapshot capability = %+v", caps)
	}
	if caps.PublicURL || caps.Upgrade || caps.Running || caps.Memory || caps.Unarchive || caps.DiskPercent || caps.PublicPreview {
		t.Fatalf("unexpected capabilities = %+v", caps)
	}
}

//nolint:gocyclo // one scenario that checks many fields of a single response
func TestCreateSendsSpec(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	if err := p.SetDefaultTemplate("team/img:1"); err != nil {
		t.Fatal(err)
	}
	createBox(t, p, func(s *sandbox.Spec) {
		s.Labels = []string{wrapperArg0, "sync"}
		s.CPU, s.ExposeAPI, s.APIPort, s.NetworkBlockAll = 4, true, 8000, true
		s.Env = []string{"A=1", "B=two"}
	})
	body := f.body("POST /v4/sandboxes", 0)
	if body["name"] != boxName || body["projectId"] != "proj" || body["image"] != "team/img:1" || body["persistent"] != true {
		t.Fatalf("body = %v", body)
	}
	if v, present := body["keepLastSnapshots"]; !present || v != nil {
		t.Fatalf("keepLastSnapshots = %v, %v", v, present)
	}
	res := asMap(body["resources"])
	if res["vcpus"] != 4.0 || res["memory"] != 8192.0 {
		t.Fatalf("resources = %v", res)
	}
	if ports := asList(body["ports"]); len(ports) != 1 || ports[0] != 8000.0 {
		t.Fatalf("ports = %v", ports)
	}
	if asMap(body["networkPolicy"])["mode"] != "deny-all" {
		t.Fatalf("networkPolicy = %v", body["networkPolicy"])
	}
	if env := asMap(body["env"]); env["A"] != "1" || env["B"] != "two" {
		t.Fatalf("env = %v", env)
	}
	tags := asMap(body["tags"])
	if tags[tagManaged] != tagTrue || tags[tagLabels] != "kaxa,sync" || tags[tagAPIPort] != "8000" {
		t.Fatalf("tags = %v", tags)
	}
}

func TestCreateMinimalBody(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	body := f.body("POST /v4/sandboxes", 0)
	for _, key := range []string{"image", "resources", "ports", "env", "networkPolicy"} {
		if _, ok := body[key]; ok {
			t.Fatalf("body has %s: %v", key, body)
		}
	}
	if f.lastQuery().Get("teamId") != "" {
		t.Fatal("personal account sent a team")
	}
}

func TestCreateRejects(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		mutate func(*sandbox.Spec)
	}{
		{keyName, func(s *sandbox.Spec) { s.Name = "Bad Name" }},
		{"template", func(s *sandbox.Spec) { s.Template = "bad template" }},
		{"cpu fraction", func(s *sandbox.Spec) { s.CPU = 1.5 }},
		{"cpu odd", func(s *sandbox.Spec) { s.CPU = 3 }},
		{"cpu negative", func(s *sandbox.Spec) { s.CPU = -2 }},
		{"api port low", func(s *sandbox.Spec) { s.ExposeAPI, s.APIPort = true, 80 }},
		{"api port reserved", func(s *sandbox.Spec) { s.ExposeAPI, s.APIPort = true, 23456 }},
		{"env", func(s *sandbox.Spec) { s.Env = []string{"NOEQUALS"} }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, f := newTestProvider(t)
			spec := sandbox.Spec{Name: boxName}
			tt.mutate(&spec)
			if _, err := p.Create(context.Background(), spec); err == nil {
				t.Fatal("Create() accepted the spec")
			}
			if f.calls("POST /v4/sandboxes") != 0 {
				t.Fatal("an invalid spec reached the API")
			}
		})
	}
}

func TestCreateErrors(t *testing.T) {
	t.Parallel()
	t.Run("name taken", func(t *testing.T) {
		t.Parallel()
		p, _ := newTestProvider(t)
		createBox(t, p)
		if _, err := p.Create(context.Background(), sandbox.Spec{Name: boxName}); !errors.Is(err, ErrAlreadyExists) {
			t.Fatalf("error = %v, want ErrAlreadyExists", err)
		}
	})
	t.Run("not configured", func(t *testing.T) {
		t.Parallel()
		if _, err := New().Create(context.Background(), sandbox.Spec{Name: boxName}); !errors.Is(err, sandbox.ErrNotConfigured) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("rejected token", func(t *testing.T) {
		t.Parallel()
		p, f := newTestProvider(t)
		f.fail2("POST /v4/sandboxes", 403)
		_, err := p.Create(context.Background(), sandbox.Spec{Name: boxName})
		if err == nil || !strings.Contains(err.Error(), "rejected") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("unmanaged answer", func(t *testing.T) {
		t.Parallel()
		p := NewWithClient(&noTagsClient{}, testToken)
		if _, err := p.Create(context.Background(), sandbox.Spec{Name: boxName}); err == nil {
			t.Fatal("Create() accepted an answer without kaxa tags")
		}
	})
}

// noTagsClient answers a create with a sandbox kaxa did not tag.
type noTagsClient struct{ apiClient }

func (*noTagsClient) Create(context.Context, CreateParams) (Record, error) {
	return Record{}, errors.New("vercel: create returned no managed sandbox")
}

func TestSetDefaultTemplateRejects(t *testing.T) {
	t.Parallel()
	if err := New().SetDefaultTemplate("a b"); err == nil {
		t.Fatal("SetDefaultTemplate() accepted a bad name")
	}
	if err := New().SetDefaultTemplate(""); err != nil {
		t.Fatal(err)
	}
}

func TestListSortsPagesAndFilters(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	f.pageSize = 1
	for _, name := range []string{"charlie", "alpha", "bravo"} {
		createBox(t, p, func(s *sandbox.Spec) { s.Name = name })
	}
	f.mu.Lock()
	f.sandboxes["foreign"] = &fakeSandbox{name: "foreign", tags: map[string]string{}, status: statusRunning}
	f.mu.Unlock()
	infos, err := p.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, i := range infos {
		got = append(got, string(i.ID))
		if i.Provider != "vercel" || i.Status != "running" || len(i.Labels) != 1 {
			t.Fatalf("info = %+v", i)
		}
	}
	if strings.Join(got, ",") != "alpha,bravo,charlie" {
		t.Fatalf("List() = %v", got)
	}
	if f.lastQuery().Get("project") != "proj" {
		t.Fatalf("query = %v", f.lastQuery())
	}
}

func TestListFailure(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	f.fail2("GET /v2/sandboxes", 429)
	if _, err := p.List(context.Background()); err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("error = %v", err)
	}
}

func TestInspect(t *testing.T) {
	t.Parallel()
	p, _ := newTestProvider(t)
	createBox(t, p, func(s *sandbox.Spec) { s.ExposeAPI, s.APIPort = true, 8000 })
	d, err := p.Inspect(context.Background(), boxName)
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != boxName || d.Provider != "vercel" || d.Status != "running" || d.URL != "https://box-8000.vercel.run" || d.Auth != sandbox.URLAuthPublic {
		t.Fatalf("Inspect() = %+v", d)
	}
	labels := map[string]bool{}
	for _, f := range d.Fields {
		labels[f.Label] = true
	}
	for _, want := range []string{"Region", "Image", "CPU", "Memory", "Ends", "URL", "Auth"} {
		if !labels[want] {
			t.Fatalf("missing field %s in %+v", want, d.Fields)
		}
	}
}

func TestInspectWithoutAPIHasNoURL(t *testing.T) {
	t.Parallel()
	p, _ := newTestProvider(t)
	createBox(t, p)
	d, err := p.Inspect(context.Background(), boxName)
	if err != nil || d.URL != "" || d.Auth != "" {
		t.Fatalf("Inspect() = %+v, %v", d, err)
	}
}

func TestLookupErrors(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	f.mu.Lock()
	f.sandboxes["foreign"] = &fakeSandbox{name: "foreign", tags: map[string]string{}, status: statusRunning}
	f.mu.Unlock()
	ctx := context.Background()
	for name, call := range map[string]func(id sandbox.ID) error{
		"inspect": func(id sandbox.ID) error { _, err := p.Inspect(ctx, id); return err },
		"destroy": func(id sandbox.ID) error { return p.Destroy(ctx, id) },
		"exec": func(id sandbox.ID) error {
			return p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{tagTrue}})
		},
		"restart": func(id sandbox.ID) error { return p.Restart(ctx, id) },
	} {
		for _, id := range []sandbox.ID{"missing", "foreign"} {
			if err := call(id); !errors.Is(err, ErrNotFound) {
				t.Errorf("%s(%s) error = %v, want ErrNotFound", name, id, err)
			}
		}
		if err := call("Bad Name"); err == nil || errors.Is(err, ErrNotFound) {
			t.Errorf("%s(bad name) error = %v", name, err)
		}
	}
	if _, ok := f.sandboxes["foreign"]; !ok {
		t.Fatal("destroy deleted a sandbox kaxa did not create")
	}
}

func TestNotConfiguredEverywhere(t *testing.T) {
	t.Parallel()
	p := New()
	ctx := context.Background()
	calls := map[string]func() error{
		"inspect": func() error { _, err := p.Inspect(ctx, boxName); return err },
		"destroy": func() error { return p.Destroy(ctx, boxName) },
		"exec":    func() error { return p.Exec(ctx, boxName, sandbox.ExecOptions{Command: []string{tagTrue}}) },
		"restart": func() error { return p.Restart(ctx, boxName) },
		"list snapshots": func() error {
			_, err := p.ListSnapshots(ctx, boxName)
			return err
		},
	}
	for name, call := range calls {
		if err := call(); !errors.Is(err, sandbox.ErrNotConfigured) {
			t.Errorf("%s error = %v", name, err)
		}
	}
}

func TestDestroy(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	if err := p.Destroy(context.Background(), boxName); err != nil {
		t.Fatal(err)
	}
	if len(f.sandboxes) != 0 {
		t.Fatal("sandbox still exists")
	}
	if f.lastQuery().Get("deleteOrphanSnapshots") != "true" {
		t.Fatalf("query = %v", f.lastQuery())
	}
}

func TestTeamScope(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ team, key string }{{"team_abc", "teamId"}, {"my-team", "slug"}} {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()
			f := newFake(t)
			p, err := NewWithBaseURL(f.srv.URL, testToken, "proj", tt.team)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := p.List(context.Background()); err != nil {
				t.Fatal(err)
			}
			other := "teamId"
			if tt.key == "teamId" {
				other = "slug"
			}
			q := f.lastQuery()
			if q.Get(tt.key) != tt.team || q.Get(other) != "" {
				t.Fatalf("query = %v", q)
			}
		})
	}
}

func TestInspectDoesNotLeakTokenInURL(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	if _, err := p.Inspect(context.Background(), boxName); err != nil {
		t.Fatal(err)
	}
	for _, q := range f.queries {
		for k, v := range q {
			if strings.Contains(k+strings.Join(v, ""), testToken) {
				t.Fatalf("token in query %v", q)
			}
		}
	}
}

func TestOpenExtendsNearExpiry(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	f.mu.Lock()
	f.sandboxes[boxName].expires = time.Now().Add(time.Minute)
	f.mu.Unlock()
	if err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{Command: []string{tagTrue}}); err != nil {
		t.Fatal(err)
	}
	if f.calls("POST /v2/sandboxes/sessions/{sid}/extend-timeout") != 1 {
		t.Fatal("a session about to expire was not extended")
	}
	f.fail2("POST /v2/sandboxes/sessions/{sid}/extend-timeout", 500)
	f.mu.Lock()
	f.sandboxes[boxName].expires = time.Now().Add(time.Minute)
	f.mu.Unlock()
	if err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{Command: []string{tagTrue}}); err == nil {
		t.Fatal("a failed extension was ignored")
	}
}

func TestOpenResumesStoppedSandbox(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	f.mu.Lock()
	f.sandboxes[boxName].status = statusStopped
	f.mu.Unlock()
	if err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{Command: []string{tagTrue}}); err != nil {
		t.Fatal(err)
	}
	if f.sandboxes[boxName].status != statusRunning {
		t.Fatal("sandbox was not resumed")
	}
}

func TestOpenWithoutSession(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p)
	f.mu.Lock()
	f.sandboxes[boxName].session = ""
	f.mu.Unlock()
	if err := p.Exec(context.Background(), boxName, sandbox.ExecOptions{Command: []string{tagTrue}}); err == nil {
		t.Fatal("Exec() ran without a session")
	}
}
