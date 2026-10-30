package docker

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

func createBox(t *testing.T, p *Provider, labels ...string) {
	t.Helper()
	if _, err := p.Create(context.Background(), sandbox.Spec{Name: testSandbox, Labels: labels}); err != nil {
		t.Fatal(err)
	}
}

func TestProviderName(t *testing.T) {
	t.Parallel()
	if got := New().Name(); got != "docker" {
		t.Fatalf("Name() = %q", got)
	}
}

func TestAvailableNeedsCredentials(t *testing.T) {
	t.Parallel()
	p := New()
	if p.Available() {
		t.Fatal("a provider without credentials must not be available")
	}
	if err := p.ConfigureCredentials("me", "pat"); err != nil {
		t.Fatal(err)
	}
	if !p.Available() {
		t.Fatal("a configured provider must be available")
	}
}

func TestConfigureCredentialsRequiresBoth(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, id, pat string }{
		{"no id", "", "pat"},
		{"no token", "me", ""},
		{"blank", "  ", "  "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := New().ConfigureCredentials(tt.id, tt.pat)
			if !errors.Is(err, sandbox.ErrNotConfigured) {
				t.Fatalf("err = %v, want ErrNotConfigured", err)
			}
			if ue, ok := usererr.As(err); !ok || !strings.Contains(ue.Hint, "KAXA_DOCKER_PAT") {
				t.Fatalf("hint = %v", ue)
			}
		})
	}
}

func TestOperationsWithoutCredentials(t *testing.T) {
	t.Parallel()
	p := New()
	ctx := context.Background()
	_, createErr := p.Create(ctx, sandbox.Spec{Name: testSandbox})
	_, listErr := p.List(ctx)
	_, inspectErr := p.Inspect(ctx, testSandbox)
	for name, err := range map[string]error{"create": createErr, "list": listErr, "inspect": inspectErr, "destroy": p.Destroy(ctx, testSandbox)} {
		if !errors.Is(err, sandbox.ErrNotConfigured) {
			t.Errorf("%s: err = %v, want ErrNotConfigured", name, err)
		}
	}
}

func TestCreateStartsSandboxAndWaitsUntilRunning(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	id, err := p.Create(context.Background(), sandbox.Spec{Name: testSandbox, Labels: []string{labelKaxa, sandbox.SyncLabel}})
	if err != nil {
		t.Fatal(err)
	}
	if id != testSandbox {
		t.Fatalf("id = %q", id)
	}
	if api.seen("get") < 2 {
		t.Fatalf("created sandbox was not polled until running: %d reads", api.seen("get"))
	}
	got := api.created[0]
	if got.Name != testSandbox || got.Image != shellImage || !slices.Equal(got.Labels, []string{labelKaxa, sandbox.SyncLabel}) {
		t.Fatalf("create params = %+v", got)
	}
}

func TestCreateImage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		fallback string
		spec     string
		want     string
	}{
		{"shell image by default", "", "", shellImage},
		{"configured default", testImage, "", testImage},
		{"flag wins", testImage, "alpine:3", "alpine:3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, api, _ := newTestProvider(t)
			if err := p.SetDefaultTemplate(tt.fallback); err != nil {
				t.Fatal(err)
			}
			if _, err := p.Create(context.Background(), sandbox.Spec{Name: testSandbox, Template: tt.spec}); err != nil {
				t.Fatal(err)
			}
			if got := api.created[0].Image; got != tt.want {
				t.Fatalf("image = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCreateRejectsBadInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		spec sandbox.Spec
	}{
		{"empty name", sandbox.Spec{}},
		{"uppercase", sandbox.Spec{Name: "Box"}},
		{"slash", sandbox.Spec{Name: "a/b"}},
		{"long", sandbox.Spec{Name: strings.Repeat("a", 64)}},
		{"template with space", sandbox.Spec{Name: testSandbox, Template: "a b"}},
		{"template traversal", sandbox.Spec{Name: testSandbox, Template: "a/../b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, api, _ := newTestProvider(t)
			if _, err := p.Create(context.Background(), tt.spec); err == nil {
				t.Fatal("expected an error")
			}
			if api.seen("create") != 0 {
				t.Fatal("API was called with invalid input")
			}
		})
	}
}

func TestCreateExistingName(t *testing.T) {
	t.Parallel()
	p, _, _ := newTestProvider(t)
	createBox(t, p)
	_, err := p.Create(context.Background(), sandbox.Spec{Name: testSandbox})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("err = %v, want ErrAlreadyExists", err)
	}
}

func TestCreateFailureRemovesSandbox(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	api.failure = "image not found"
	_, err := p.Create(context.Background(), sandbox.Spec{Name: testSandbox})
	if err == nil || !strings.Contains(err.Error(), "image not found") {
		t.Fatalf("err = %v, want the reason Docker gave", err)
	}
	if api.seen("delete") != 1 {
		t.Fatal("a sandbox that failed to start must be deleted")
	}
}

func TestCreateAPIErrorIsRedacted(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	api.failOn["create"] = errors.New("boom " + testPAT)
	_, err := p.Create(context.Background(), sandbox.Spec{Name: testSandbox})
	if err == nil || strings.Contains(err.Error(), testPAT) {
		t.Fatalf("err = %v, must not hold the token", err)
	}
}

func TestCreateTimesOut(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := NewWithClients(api, &fakeStarter{}, time.Millisecond, 20*time.Millisecond)
	// A sandbox that stays in "starting" never runs.
	api.stuck = true
	_, err := p.Create(context.Background(), sandbox.Spec{Name: testSandbox})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want a deadline error", err)
	}
}

func TestListShowsOwnSandboxesSorted(t *testing.T) {
	t.Parallel()
	p, _, _ := newTestProvider(t)
	for _, name := range []string{"b", "a"} {
		if _, err := p.Create(context.Background(), sandbox.Spec{Name: name, Labels: []string{labelKaxa}}); err != nil {
			t.Fatal(err)
		}
	}
	infos, err := p.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 2 || infos[0].ID != "a" || infos[1].ID != "b" {
		t.Fatalf("infos = %+v", infos)
	}
	if infos[0].Provider != "docker" || !slices.Equal(infos[0].Labels, []string{labelKaxa}) {
		t.Fatalf("info = %+v", infos[0])
	}
}

func TestInspect(t *testing.T) {
	t.Parallel()
	p, _, _ := newTestProvider(t)
	createBox(t, p, labelKaxa)
	d, err := p.Inspect(context.Background(), testSandbox)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != statusRunning || d.Provider != "docker" || d.CreatedAt.IsZero() || d.URL != "" {
		t.Fatalf("detail = %+v", d)
	}
}

func TestInspectShowsThePublishedURL(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	createBox(t, p, labelKaxa)
	api.mu.Lock()
	for _, r := range api.recs {
		r.Ports = []PublishedPort{{Port: 9000}, {Port: 8000, URL: "https://8000-box.sbx.example.test"}}
	}
	api.mu.Unlock()
	d, err := p.Inspect(context.Background(), testSandbox)
	if err != nil {
		t.Fatal(err)
	}
	if d.URL != "https://8000-box.sbx.example.test" {
		t.Fatalf("URL = %q", d.URL)
	}
}

func TestInspectUnknown(t *testing.T) {
	t.Parallel()
	p, _, _ := newTestProvider(t)
	_, err := p.Inspect(context.Background(), "nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if ue, ok := usererr.As(err); !ok || !strings.Contains(ue.Hint, "kaxa list") {
		t.Fatalf("hint = %v", ue)
	}
}

func TestDestroyWaitsUntilGone(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	createBox(t, p)
	if err := p.Destroy(context.Background(), testSandbox); err != nil {
		t.Fatal(err)
	}
	infos, err := p.List(context.Background())
	if err != nil || len(infos) != 0 {
		t.Fatalf("infos = %v, err = %v", infos, err)
	}
	if api.seen("delete") != 1 {
		t.Fatal("expected one delete")
	}
}

func TestDestroyUnknown(t *testing.T) {
	t.Parallel()
	p, _, _ := newTestProvider(t)
	if err := p.Destroy(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestFindRejectsInvalidName(t *testing.T) {
	t.Parallel()
	p, api, _ := newTestProvider(t)
	if _, err := p.Inspect(context.Background(), "../x"); err == nil {
		t.Fatal("expected an error")
	}
	if api.seen("find") != 0 {
		t.Fatal("API was called with an invalid name")
	}
}

func TestUserErrMapsAPIErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		err      error
		wantMsg  string
		wantHint string
	}{
		{"unauthenticated", &APIError{Status: 401, Code: "unauthenticated"}, "rejected the credentials", "KAXA_DOCKER_PAT"},
		{"forbidden", &APIError{Status: 403}, "denied access", "subscription"},
		{"quota", &APIError{Status: 429, Code: "resourceExhausted", Message: "10 sandboxes"}, "limit reached", "destroy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ue, ok := usererr.As(userErr(tt.err, testSandbox))
			if !ok {
				t.Fatal("want a user error")
			}
			if !strings.Contains(ue.Message, tt.wantMsg) || !strings.Contains(ue.Hint, tt.wantHint) {
				t.Fatalf("got %q / %q", ue.Message, ue.Hint)
			}
		})
	}
}

func TestAPIErrorMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		err  APIError
		want string
	}{
		{APIError{Code: "notFound", Message: "gone"}, "docker: notFound: gone"},
		{APIError{Code: "internal"}, "docker: internal"},
		{APIError{Status: 502, Message: "bad gateway"}, "docker: API returned status 502: bad gateway"},
		{APIError{Status: 500}, "docker: API returned status 500"},
	}
	for _, tt := range tests {
		if got := tt.err.Error(); got != tt.want {
			t.Errorf("got %q, want %q", got, tt.want)
		}
	}
}
