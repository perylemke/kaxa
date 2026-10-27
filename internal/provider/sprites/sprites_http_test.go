package sprites_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/kaxa/internal/provider/sprites"
	"github.com/perylemke/kaxa/internal/sandbox"
)

func assertLifecycleList(t *testing.T, list []sandbox.Info) {
	t.Helper()
	if len(list) != 2 {
		t.Fatalf("len(list) = %d, want 2", len(list))
	}
	if list[0].Status != spriteStatusCold || list[1].Status != spriteStatusWarm {
		t.Fatalf("statuses = %q, %q", list[0].Status, list[1].Status)
	}
	if list[0].Provider != spritesName || list[1].Provider != spritesName {
		t.Fatalf("provider = %q, %q", list[0].Provider, list[1].Provider)
	}
	if len(list[0].Labels) != 1 || list[0].Labels[0] != "team=a" {
		t.Fatalf("labels[0] = %v", list[0].Labels)
	}
}

type lifecycleProbe struct {
	client        *fakeSDKClient
	createdName   string
	createdLabels []string
	getCalls      int
	destroyed     string
}

func newLifecycleProbe(t *testing.T, token string) *lifecycleProbe {
	t.Helper()
	probe := &lifecycleProbe{}
	probe.client = &fakeSDKClient{
		create: func(_ context.Context, name string, labels []string) (*spritesdk.Sprite, error) {
			if name != lifecycleSpriteName || len(labels) != 1 || labels[0] != lifecycleKaxaLabel {
				t.Fatalf("create name=%q labels=%v", name, labels)
			}
			probe.createdName = name
			probe.createdLabels = append([]string(nil), labels...)
			return spritesdk.New(token).Sprite(lifecycleSpriteName), nil
		},
		get: func(_ context.Context, _ string) (*spritesdk.Sprite, error) {
			probe.getCalls++
			status := spriteStatusCold
			if probe.getCalls >= 2 {
				status = spriteStatusWarm
			}
			return &spritesdk.Sprite{Status: status}, nil
		},
		list: func(_ context.Context, _ string) ([]sprites.SpriteListEntry, error) {
			return []sprites.SpriteListEntry{
				{Name: "alpha", Status: spriteStatusCold, Labels: []string{"team=a"}},
				{Name: "beta", Status: spriteStatusWarm},
			}, nil
		},
		destroy: func(_ context.Context, name string) error {
			probe.destroyed = name
			return nil
		},
	}
	return probe
}

func TestHTTPLifecycle(t *testing.T) {
	const token = "httptest-token-value" // #nosec G101 -- test fixture
	probe := newLifecycleProbe(t, token)
	p := sprites.NewWithClient(probe.client, token)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	id, err := p.Create(ctx, sandbox.Spec{Name: lifecycleSpriteName, Labels: []string{lifecycleKaxaLabel}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id != lifecycleSpriteName {
		t.Fatalf("id = %q", id)
	}
	if probe.createdName != lifecycleSpriteName || len(probe.createdLabels) != 1 || probe.createdLabels[0] != lifecycleKaxaLabel {
		t.Fatalf("create payload name=%q labels=%v", probe.createdName, probe.createdLabels)
	}
	if probe.getCalls < 2 {
		t.Fatalf("getCalls = %d, want at least 2 warm polls", probe.getCalls)
	}

	list, err := p.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	assertLifecycleList(t, list)

	if err := p.Destroy(ctx, "alpha"); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if probe.destroyed != "alpha" {
		t.Fatalf("destroyed = %q", probe.destroyed)
	}
}

func TestHTTPErrorRedactsToken(t *testing.T) {
	const token = "secret-in-error-body" // #nosec G101 -- test fixture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "failed with "+token, http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	p := sprites.New()
	if err := p.ConfigureWithOptions(token, srv.URL, spritesdk.WithDisableControl()); err != nil {
		t.Fatalf("configure: %v", err)
	}

	_, err := p.Create(context.Background(), sandbox.Spec{Name: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("error leaked token: %v", err)
	}
}
