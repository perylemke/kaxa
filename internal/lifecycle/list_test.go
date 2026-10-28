package lifecycle_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/config"
	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	listSprites = "sprites"
	listModal   = "modal"
)

// listProvider is a provider whose configuration and List result are scripted.
type listProvider struct {
	altProvider
	infos        []sandbox.Info
	listErr      error
	notConfigure bool
}

func newListProvider(name string) *listProvider {
	l := &listProvider{}
	l.name = name
	return l
}

func (l *listProvider) Configure(string) error {
	if l.notConfigure {
		return sandbox.ErrNotConfigured
	}
	return nil
}

func (l *listProvider) ConfigureCredentials(string, string) error {
	return l.Configure("")
}

func (l *listProvider) List(context.Context) ([]sandbox.Info, error) {
	return l.infos, l.listErr
}

func TestListTargetsSkipsUnconfiguredAndUnavailable(t *testing.T) {
	t.Parallel()

	unconfigured := newListProvider(listModal)
	unconfigured.notConfigure = true
	unavailable := newListProvider("stub")
	unavailable.unavailable = true
	reg := registryWith(t, newListProvider(listSprites), unconfigured, unavailable)

	providers, names, err := lifecycle.ListTargets(reg, config.Config{})
	if err != nil {
		t.Fatalf("list targets: %v", err)
	}
	if !slices.Equal(names, []string{listSprites}) || len(providers) != 1 || providers[listSprites] == nil {
		t.Fatalf("names = %v providers = %v", names, providers)
	}
}

func TestListTargetsNoneConfigured(t *testing.T) {
	t.Parallel()

	unconfigured := newListProvider(listModal)
	unconfigured.notConfigure = true
	reg := registryWith(t, unconfigured)

	_, _, err := lifecycle.ListTargets(reg, config.Config{})
	ue, ok := usererr.As(err)
	if !ok || ue.Message != "no provider is configured" || !strings.Contains(ue.Hint, "KAXA_MODAL_TOKEN_ID") {
		t.Fatalf("err = %v", err)
	}
}

func TestCollectSandboxes(t *testing.T) {
	t.Parallel()

	okSprites := newListProvider(listSprites)
	okSprites.infos = []sandbox.Info{{ID: "a", Status: "warm"}}
	okModal := newListProvider(listModal)
	okModal.infos = []sandbox.Info{{ID: "b", Provider: listModal, Status: "running"}}
	broken := newListProvider(listModal)
	broken.listErr = errors.New("boom")

	tests := []struct {
		name         string
		providers    []*listProvider
		wantIDs      []string
		wantFailures int
		wantErr      bool
	}{
		{name: "all providers in order", providers: []*listProvider{okSprites, okModal}, wantIDs: []string{"a", "b"}},
		{name: "one failure is a warning", providers: []*listProvider{okSprites, broken}, wantIDs: []string{"a"}, wantFailures: 1},
		{name: "all failing is an error", providers: []*listProvider{broken}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			byName := map[string]sandbox.Provider{}
			var names []string
			for _, p := range tc.providers {
				byName[p.Name()] = p
				names = append(names, p.Name())
			}
			infos, failures, err := lifecycle.CollectSandboxes(context.Background(), byName, names)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			var ids []string
			for _, info := range infos {
				ids = append(ids, string(info.ID))
				if info.Provider == "" {
					t.Fatalf("info %q has no provider", info.ID)
				}
			}
			if !slices.Equal(ids, tc.wantIDs) || len(failures) != tc.wantFailures {
				t.Fatalf("ids = %v failures = %v", ids, failures)
			}
			for _, f := range failures {
				if !strings.Contains(f.Error(), "modal: boom") {
					t.Fatalf("failure = %v", f)
				}
			}
		})
	}
}
