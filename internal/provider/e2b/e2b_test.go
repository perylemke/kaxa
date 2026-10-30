package e2b_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/provider/e2b"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

func TestProviderName(t *testing.T) {
	t.Parallel()

	if got := e2b.New().Name(); got != "e2b" {
		t.Fatalf("name = %q", got)
	}
	if err := registry.New().Register(e2b.New()); err != nil {
		t.Fatalf("register: %v", err)
	}
}

func TestConfigureAndAvailability(t *testing.T) {
	t.Parallel()

	p := e2b.New()
	if sandbox.IsAvailable(p) {
		t.Fatal("provider without a key is available")
	}
	err := p.Configure("")
	ue, ok := usererr.As(err)
	if !errors.Is(err, sandbox.ErrNotConfigured) || !ok || !strings.Contains(ue.Hint, "KAXA_E2B_API_KEY") {
		t.Fatalf("err = %v", err)
	}
	if sandbox.IsAvailable(p) {
		t.Fatal("provider with an empty key is available")
	}
	if err := p.Configure("key"); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if err := p.Configure("key"); err != nil {
		t.Fatalf("configure again: %v", err)
	}
	if !sandbox.IsAvailable(p) {
		t.Fatal("configured provider is not available")
	}
}

func TestMethodsWithoutKeyAreNotConfigured(t *testing.T) {
	t.Parallel()

	p := e2b.New()
	ctx := context.Background()
	calls := map[string]func() error{
		"create":  func() error { _, err := p.Create(ctx, sandbox.Spec{Name: testBox}); return err },
		"list":    func() error { _, err := p.List(ctx); return err },
		"inspect": func() error { _, err := p.Inspect(ctx, testBox); return err },
		"destroy": func() error { return p.Destroy(ctx, testBox) },
		"exec": func() error {
			return p.Exec(ctx, testBox, sandbox.ExecOptions{Command: []string{"ls"}})
		},
		"restart":         func() error { return p.Restart(ctx, testBox) },
		sandbox.SyncLabel: func() error { return p.SyncWorkspace(ctx, testBox, strings.NewReader("x")) },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := call(); !errors.Is(err, sandbox.ErrNotConfigured) {
				t.Fatalf("err = %v, want ErrNotConfigured", err)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		spec       sandbox.Spec
		want       e2b.CreateParams
		wantErrHas string
	}{
		{
			name: "private by default",
			spec: sandbox.Spec{Name: testApp, Labels: []string{testKaxa, sandbox.SyncLabel}},
			want: e2b.CreateParams{Name: testApp, Labels: []string{testKaxa, sandbox.SyncLabel}},
		},
		{
			name: "api is public on its port",
			spec: sandbox.Spec{Name: testApp, ExposeAPI: true, APIPort: 8000},
			want: e2b.CreateParams{Name: testApp, APIPort: 8000, PublicTraffic: true},
		},
		{name: "invalid name", spec: sandbox.Spec{Name: "Bad_Name"}, wantErrHas: "invalid sandbox name"},
		{name: "taken name", spec: sandbox.Spec{Name: testBox}, wantErrHas: "already exists"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, api, _ := newProvider(nil, 0)
			id, err := p.Create(context.Background(), tt.spec)
			if tt.wantErrHas != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrHas) {
					t.Fatalf("err = %v, want %q", err, tt.wantErrHas)
				}
				if len(api.creates) != 0 {
					t.Fatalf("created %v despite the error", api.creates)
				}
				return
			}
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			if id != sandbox.ID(tt.spec.Name) || len(api.creates) != 1 {
				t.Fatalf("id = %q creates = %+v", id, api.creates)
			}
			got := api.creates[0]
			if got.Name != tt.want.Name || got.APIPort != tt.want.APIPort || got.PublicTraffic != tt.want.PublicTraffic || !slices.Equal(got.Labels, tt.want.Labels) {
				t.Fatalf("params = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCreateTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		defaultName string
		spec        string
		want        string
	}{
		{name: "none means base", want: ""},
		{name: "spec template", spec: testBig, want: testBig},
		{name: "default template", defaultName: testTeam, want: testTeam},
		{name: "spec wins over default", defaultName: testTeam, spec: testBig, want: testBig},
		{name: "explicit base is base", defaultName: testTeam, spec: "base", want: ""},
		{name: "default base is base", defaultName: "base", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, api, _ := newProvider(nil, 0)
			if err := p.SetDefaultTemplate(tt.defaultName); err != nil {
				t.Fatalf("set default: %v", err)
			}
			if _, err := p.Create(context.Background(), sandbox.Spec{Name: testApp, Template: tt.spec}); err != nil {
				t.Fatalf("create: %v", err)
			}
			if got := api.creates[0].Template; got != tt.want {
				t.Fatalf("template = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCreateRejectsBadTemplate(t *testing.T) {
	t.Parallel()

	for _, bad := range []string{"has space", "-lead", "../x", "a..b", "x;rm", strings.Repeat("a", 129), "a\nb"} {
		t.Run(bad, func(t *testing.T) {
			t.Parallel()
			p, api, _ := newProvider(nil, 0)
			_, err := p.Create(context.Background(), sandbox.Spec{Name: testApp, Template: bad})
			if err == nil || !strings.Contains(err.Error(), "invalid template") {
				t.Fatalf("err = %v", err)
			}
			if len(api.creates) != 0 {
				t.Fatalf("created %v despite the error", api.creates)
			}
			if err := p.SetDefaultTemplate(bad); err == nil {
				t.Fatal("bad default template accepted")
			}
		})
	}
}

func TestCreateTemplateNotFound(t *testing.T) {
	t.Parallel()

	p, api, _ := newProvider(nil, 0)
	api.createErr = e2b.ErrTemplateNotFound
	_, err := p.Create(context.Background(), sandbox.Spec{Name: testApp, Template: "nope"})
	if err == nil || !strings.Contains(err.Error(), `template "nope" not found`) {
		t.Fatalf("err = %v", err)
	}
}

func TestProviderSupportsTemplate(t *testing.T) {
	t.Parallel()

	p, _, _ := newProvider(nil, 0)
	if !sandbox.CapabilitiesOf(p).Template {
		t.Fatal("e2b does not report the template capability")
	}
}

func TestListSortsByNameAndReportsState(t *testing.T) {
	t.Parallel()

	p, api, _ := newProvider([]string{testKaxa}, 0)
	api.add("alpha", "paused", []string{testKaxa, sandbox.SyncLabel}, 0)

	got, err := p.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 || got[0].ID != "alpha" || got[1].ID != testBox {
		t.Fatalf("infos = %+v", got)
	}
	if got[0].Status != "paused" || got[0].Provider != "e2b" || !slices.Contains(got[0].Labels, sandbox.SyncLabel) || got[1].Status != "running" {
		t.Fatalf("infos = %+v", got)
	}
}

func TestInspect(t *testing.T) {
	t.Parallel()

	p, api, _ := newProvider([]string{testKaxa}, 0)
	api.add("api", "running", []string{testKaxa}, 8000)

	plain, err := p.Inspect(context.Background(), testBox)
	if err != nil || plain.URL != "" || plain.Status != "running" || plain.Provider != "e2b" {
		t.Fatalf("detail = %+v err = %v", plain, err)
	}
	pub, err := p.Inspect(context.Background(), "api")
	if err != nil {
		t.Fatalf("inspect api: %v", err)
	}
	wantURL := "https://8000-" + api.boxes["api"].ID + ".e2b.app"
	if pub.URL != wantURL || pub.Auth != sandbox.URLAuthPublic {
		t.Fatalf("detail = %+v, want url %q", pub, wantURL)
	}
	if _, err := p.Inspect(context.Background(), "missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing err = %v", err)
	}
}

func TestDestroy(t *testing.T) {
	t.Parallel()

	p, api, _ := newProvider(nil, 0)
	id := api.boxes[testBox].ID
	if err := p.Destroy(context.Background(), testBox); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if !slices.Equal(api.killed, []string{id}) {
		t.Fatalf("killed = %v", api.killed)
	}
	if err := p.Destroy(context.Background(), testBox); !errors.Is(err, e2b.ErrNotFound) {
		t.Fatalf("second destroy err = %v", err)
	}
}

func TestErrorsNeverContainSecrets(t *testing.T) {
	t.Parallel()

	calls := map[string]func(*e2b.Provider) error{
		"create": func(p *e2b.Provider) error {
			_, err := p.Create(context.Background(), sandbox.Spec{Name: "other"})
			return err
		},
		"list":    func(p *e2b.Provider) error { _, err := p.List(context.Background()); return err },
		"inspect": func(p *e2b.Provider) error { _, err := p.Inspect(context.Background(), testBox); return err },
		"destroy": func(p *e2b.Provider) error { return p.Destroy(context.Background(), testBox) },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p, api, _ := newProvider(nil, 0)
			api.failWith = errors.New("request failed with key " + testKey)
			err := call(p)
			if err == nil {
				t.Fatal("expected an error")
			}
			if strings.Contains(err.Error(), testKey) {
				t.Fatalf("error leaks the key: %v", err)
			}
		})
	}
}

func TestRejectedKeyIsExplained(t *testing.T) {
	t.Parallel()

	p, api, _ := newProvider(nil, 0)
	api.failWith = &e2b.APIError{Status: 401, Message: "invalid key " + testKey}
	_, err := p.List(context.Background())
	ue, ok := usererr.As(err)
	if !ok || ue.Message != "E2B rejected the API key" || !strings.Contains(ue.Hint, "KAXA_E2B_API_KEY") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(ue.Error(), testKey) || (ue.Cause != nil && strings.Contains(ue.Cause.Error(), testKey)) {
		t.Fatalf("error leaks the key: %v", err)
	}
}
