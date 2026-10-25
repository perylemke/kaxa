package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/perylemke/sbox/internal/sandbox"
)

func TestListInteractiveDetailViewShowsFields(t *testing.T) {
	t.Parallel()

	m := listInteractiveModel{
		mode: listModeDetail,
		detail: sandbox.Detail{
			ID:           fixtureDemo,
			Status:       fixtureWarm,
			SpriteUUID:   "uuid-1",
			Organization: "acme",
			URL:          "https://demo.sprites.dev",
			Auth:         "sprite · admins",
			Labels:       []string{"sbox", "team=a"},
			CreatedAt:    time.Now().Add(-2 * time.Hour),
			UpdatedAt:    time.Now().Add(-5 * time.Minute),
		},
	}
	view := stripANSI(m.View())
	for _, want := range []string{
		fixtureDemo,
		fixtureWarm,
		"sprite-uuid-1",
		"acme",
		"https://demo.sprites.dev",
		"sprite · admins",
		"sbox,team=a",
		"r restart",
		"enter auth",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestListInteractiveListViewShowsHelp(t *testing.T) {
	t.Parallel()

	m := newListInteractiveModel(context.TODO(), nil, []sandbox.Info{
		{ID: "a", Status: fixtureWarm, Provider: spritesProviderName},
	})
	m.mode = listModeTable
	view := stripANSI(m.View())
	if !strings.Contains(view, "enter detail") || !strings.Contains(view, "a") {
		t.Fatalf("view = %q", view)
	}
}

func TestListEnterOpensURLAuth(t *testing.T) {
	t.Parallel()

	m := listInteractiveModel{
		mode:   listModeDetail,
		detail: sandbox.Detail{ID: fixtureDemo, Auth: "sprite · admins"},
	}
	next, cmd := m.updateDetailKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := authModel(t, next)
	if got.postAction != listPostURLAuth || got.postID != fixtureDemo || got.auth != sandbox.URLAuthSprite {
		t.Fatalf("action=%v id=%q auth=%q", got.postAction, got.postID, got.auth)
	}
	if cmd == nil {
		t.Fatal("expected quit")
	}
}

func authModel(t *testing.T, model tea.Model) listInteractiveModel {
	t.Helper()
	got, ok := model.(listInteractiveModel)
	if !ok {
		t.Fatalf("model = %T", model)
	}
	return got
}

func TestRunListURLAuthSetsAuth(t *testing.T) {
	t.Parallel()

	p := &recordingURLProvider{detail: sandbox.Detail{URL: "https://demo.sprites.app"}}
	out := new(bytes.Buffer)
	err := runListURLAuth(context.Background(), strings.NewReader("1\ny\n"), new(bytes.Buffer), out, p, listTUIResult{
		id:   fixtureDemo,
		auth: sandbox.URLAuthSprite,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if p.auth != sandbox.URLAuthPublic {
		t.Fatalf("auth = %q", p.auth)
	}
	if !strings.Contains(out.String(), "https://demo.sprites.app") || !strings.Contains(out.String(), sandbox.URLAuthPublic) {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunListURLAuthCancel(t *testing.T) {
	t.Parallel()

	p := &recordingURLProvider{}
	out := new(bytes.Buffer)
	err := runListURLAuth(context.Background(), strings.NewReader("\nn\n"), new(bytes.Buffer), out, p, listTUIResult{
		id:   fixtureDemo,
		auth: sandbox.URLAuthPublic,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if p.called {
		t.Fatal("auth was changed")
	}
	if !strings.Contains(out.String(), "cancelled") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunListURLAuthRejectsProvider(t *testing.T) {
	t.Parallel()

	err := runListURLAuth(context.Background(), strings.NewReader("y\n"), new(bytes.Buffer), new(bytes.Buffer), idleProvider{}, listTUIResult{
		id:   fixtureDemo,
		auth: sandbox.URLAuthPublic,
	})
	if err == nil || !strings.Contains(err.Error(), "only supported") {
		t.Fatalf("err = %v", err)
	}
}

type idleProvider struct{}

func (idleProvider) Name() string { return spritesProviderName }

func (idleProvider) Create(context.Context, sandbox.Spec) (sandbox.ID, error) { return "", nil }

func (idleProvider) List(context.Context) ([]sandbox.Info, error) { return nil, nil }

func (idleProvider) Inspect(context.Context, sandbox.ID) (sandbox.Detail, error) {
	return sandbox.Detail{}, nil
}

func (idleProvider) Restart(context.Context, sandbox.ID) error { return nil }

func (idleProvider) Upgrade(context.Context, sandbox.ID) error { return nil }

func (idleProvider) Exec(context.Context, sandbox.ID, sandbox.ExecOptions) error { return nil }

func (idleProvider) Destroy(context.Context, sandbox.ID) error { return nil }

type recordingURLProvider struct {
	idleProvider
	auth   string
	called bool
	detail sandbox.Detail
}

func (p *recordingURLProvider) Inspect(context.Context, sandbox.ID) (sandbox.Detail, error) {
	return p.detail, nil
}

func (p *recordingURLProvider) SetURLAuth(_ context.Context, id sandbox.ID, auth string) error {
	if id != fixtureDemo {
		return errors.New("unexpected sandbox")
	}
	p.called = true
	p.auth = auth
	return nil
}
