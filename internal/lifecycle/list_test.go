package lifecycle_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

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
	// started and proceed hold List until every provider has started.
	started chan struct{}
	proceed chan struct{}
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
	if l.started != nil {
		l.started <- struct{}{}
		select {
		case <-l.proceed:
		case <-time.After(5 * time.Second):
			return nil, errors.New("the other provider never started: the listing is not concurrent")
		}
	}
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

func TestCollectSandboxesListsProvidersAtTheSameTime(t *testing.T) {
	t.Parallel()

	started := make(chan struct{}, 2)
	proceed := make(chan struct{})
	first, second := newListProvider(listSprites), newListProvider(listModal)
	first.infos = []sandbox.Info{{ID: "a"}}
	second.infos = []sandbox.Info{{ID: "b"}}
	for _, p := range []*listProvider{first, second} {
		p.started, p.proceed = started, proceed
	}
	go func() {
		for range 2 {
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				return
			}
		}
		close(proceed)
	}()

	byName := map[string]sandbox.Provider{listSprites: first, listModal: second}
	infos, failures, err := lifecycle.CollectSandboxes(context.Background(), byName, []string{listSprites, listModal})
	if err != nil || len(failures) != 0 || len(infos) != 2 {
		t.Fatalf("infos = %v failures = %v err = %v", infos, failures, err)
	}
}

func TestCollectSandboxesKeepsTheOrderOfNames(t *testing.T) {
	t.Parallel()

	slow, fast := newListProvider(listSprites), newListProvider(listModal)
	slow.infos = []sandbox.Info{{ID: "slow"}}
	fast.infos = []sandbox.Info{{ID: "fast"}}
	// fast starts and ends first; slow waits for it, then finishes.
	slow.started, slow.proceed = make(chan struct{}, 1), make(chan struct{})
	close(slow.proceed)

	byName := map[string]sandbox.Provider{listSprites: slow, listModal: fast}
	infos, _, err := lifecycle.CollectSandboxes(context.Background(), byName, []string{listSprites, listModal})
	if err != nil || len(infos) != 2 || infos[0].ID != "slow" || infos[1].ID != "fast" {
		t.Fatalf("infos = %v err = %v", infos, err)
	}
}
