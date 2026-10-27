package modal_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/provider/modal"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	testTokenID     = "ak-test-id"
	testTokenSecret = "as-test-secret" // #nosec G101 -- fake value for tests
	testName        = "demo"
	testSandboxID   = "sb-1"
)

func newProvider(t *testing.T, fake *fakeClient) *modal.Provider {
	t.Helper()
	return modal.NewWithClient(fake, testTokenID, testTokenSecret)
}

func TestNameAndAvailability(t *testing.T) {
	t.Parallel()

	p := modal.New()
	if p.Name() != testProviderName {
		t.Fatalf("name = %q", p.Name())
	}
	if !sandbox.IsAvailable(p) {
		t.Fatal("modal should be available")
	}
}

func TestCapabilities(t *testing.T) {
	t.Parallel()

	if got := sandbox.CapabilitiesOf(modal.New()); got != (sandbox.Capabilities{}) {
		t.Fatalf("capabilities = %+v, want none", got)
	}
}

func TestConfigureCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, id, secret string
	}{
		{"missing id", "", testTokenSecret},
		{"missing secret", testTokenID, ""},
		{"missing both", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := modal.New().ConfigureCredentials(tt.id, tt.secret)
			if !errors.Is(err, modal.ErrMissingCredentials) {
				t.Fatalf("err = %v, want ErrMissingCredentials", err)
			}
			ue, ok := usererr.As(err)
			if !ok || !strings.Contains(ue.Hint, "KAXA_MODAL_TOKEN_ID") || !strings.Contains(ue.Hint, "KAXA_MODAL_TOKEN_SECRET") {
				t.Fatalf("hint = %+v", ue)
			}
		})
	}
}

func TestMethodsFailWithoutCredentials(t *testing.T) {
	t.Parallel()

	p := modal.New()
	ctx := context.Background()
	if _, err := p.Create(ctx, sandbox.Spec{Name: testName}); !errors.Is(err, modal.ErrMissingCredentials) {
		t.Fatalf("create err = %v", err)
	}
	if _, err := p.List(ctx); !errors.Is(err, modal.ErrMissingCredentials) {
		t.Fatalf("list err = %v", err)
	}
	if _, err := p.Inspect(ctx, testName); !errors.Is(err, modal.ErrMissingCredentials) {
		t.Fatalf("inspect err = %v", err)
	}
	if err := p.Destroy(ctx, testName); !errors.Is(err, modal.ErrMissingCredentials) {
		t.Fatalf("destroy err = %v", err)
	}
}

func TestCreate(t *testing.T) {
	t.Parallel()

	fake := newFakeClient()
	p := newProvider(t, fake)

	id, err := p.Create(context.Background(), sandbox.Spec{Name: testName, Labels: []string{testKaxa, "sync"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id != testName {
		t.Fatalf("id = %q", id)
	}
	if got := fake.boxes[testName]; got == nil || len(got.Labels) != 2 {
		t.Fatalf("stored = %+v", got)
	}
}

func TestCreateValidatesName(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"", "..", "a/b", "UPPER", "-lead", strings.Repeat("a", 64)} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fake := newFakeClient()
			_, err := newProvider(t, fake).Create(context.Background(), sandbox.Spec{Name: name})
			if _, ok := usererr.As(err); !ok {
				t.Fatalf("err = %v, want user error", err)
			}
			if fake.creates != 0 {
				t.Fatal("client called for an invalid name")
			}
		})
	}
}

func TestCreateRejectsDuplicate(t *testing.T) {
	t.Parallel()

	fake := newFakeClient()
	fake.boxes[testName] = &modal.Record{ID: testSandboxID, Name: testName}
	p := newProvider(t, fake)

	_, err := p.Create(context.Background(), sandbox.Spec{Name: testName})
	ue, ok := usererr.As(err)
	if !ok || !strings.Contains(ue.Message, "already exists") {
		t.Fatalf("err = %v", err)
	}
}

func TestListSortedWithStatusAndLabels(t *testing.T) {
	t.Parallel()

	code := 0
	fake := newFakeClient()
	fake.boxes["b"] = &modal.Record{ID: "sb-b", Name: "b", Labels: []string{testKaxa}}
	fake.boxes["a"] = &modal.Record{ID: "sb-a", Name: "a", ExitCode: &code}

	infos, err := newProvider(t, fake).List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(infos) != 2 || infos[0].ID != "a" || infos[1].ID != "b" {
		t.Fatalf("infos = %+v", infos)
	}
	if infos[0].Status != "exited (0)" || infos[1].Status != "running" {
		t.Fatalf("statuses = %q, %q", infos[0].Status, infos[1].Status)
	}
	if infos[0].Provider != testProviderName || len(infos[1].Labels) != 1 {
		t.Fatalf("infos = %+v", infos)
	}
}

func TestListEmpty(t *testing.T) {
	t.Parallel()

	infos, err := newProvider(t, newFakeClient()).List(context.Background())
	if err != nil || len(infos) != 0 {
		t.Fatalf("infos = %+v err = %v", infos, err)
	}
}

func TestInspect(t *testing.T) {
	t.Parallel()

	fake := newFakeClient()
	fake.boxes[testName] = &modal.Record{ID: testSandboxID, Name: testName, Labels: []string{testKaxa}}
	p := newProvider(t, fake)

	d, err := p.Inspect(context.Background(), testName)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if d.ID != testName || d.Provider != "modal" || d.Status != "running" || len(d.Labels) != 1 {
		t.Fatalf("detail = %+v", d)
	}

	_, err = p.Inspect(context.Background(), "missing")
	ue, ok := usererr.As(err)
	if !ok || !strings.Contains(ue.Message, "not found") {
		t.Fatalf("missing err = %v", err)
	}
}

func TestInspectTreatsFinishedSandboxAsNotFound(t *testing.T) {
	t.Parallel()

	code := 0
	fake := newFakeClient()
	fake.boxes[testName] = &modal.Record{ID: testSandboxID, Name: testName, ExitCode: &code}

	_, err := newProvider(t, fake).Inspect(context.Background(), testName)
	if !errors.Is(err, modal.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDestroy(t *testing.T) {
	t.Parallel()

	fake := newFakeClient()
	fake.boxes[testName] = &modal.Record{ID: testSandboxID, Name: testName}
	p := newProvider(t, fake)

	if err := p.Destroy(context.Background(), testName); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if _, ok := fake.boxes[testName]; ok {
		t.Fatal("sandbox still present")
	}
	err := p.Destroy(context.Background(), testName)
	if ue, ok := usererr.As(err); !ok || !strings.Contains(ue.Message, "not found") {
		t.Fatalf("second destroy err = %v", err)
	}
}

func TestErrorsRedactCredentials(t *testing.T) {
	t.Parallel()

	fake := newFakeClient()
	fake.failWith = errors.New("boom " + testTokenSecret + " and " + testTokenID)
	p := newProvider(t, fake)

	_, err := p.List(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), testTokenSecret) || strings.Contains(err.Error(), testTokenID) {
		t.Fatalf("error leaks credentials: %v", err)
	}
}
