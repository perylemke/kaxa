package cli_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/cli"
	"github.com/perylemke/kaxa/internal/provider/sprites"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	testModalName = "modal"
	listToken     = "tok"
)

type listSprites struct {
	coreProvider
	infos []sandbox.Info
	err   error
}

func (l *listSprites) Configure(token string) error {
	if token == "" {
		return sprites.ErrMissingToken
	}
	return nil
}

func (l *listSprites) List(context.Context) ([]sandbox.Info, error) { return l.infos, l.err }

type listModal struct {
	coreProvider
	infos []sandbox.Info
	err   error
}

func (l *listModal) ConfigureCredentials(id, secret string) error {
	if id == "" || secret == "" {
		return usererr.Wrap(sandbox.ErrNotConfigured, "missing", "")
	}
	return nil
}

func (l *listModal) List(context.Context) ([]sandbox.Info, error) { return l.infos, l.err }

type listEnv struct{ spritesToken, modalID, modalSecret, defaultProvider string }

func runList(t *testing.T, env listEnv, sp *listSprites, md *listModal, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	reg := registry.New()
	for _, p := range []sandbox.Provider{sp, md} {
		if regErr := reg.Register(p); regErr != nil {
			t.Fatalf("register: %v", regErr)
		}
	}
	t.Setenv("KAXA_SPRITES_TOKEN", env.spritesToken)
	t.Setenv("KAXA_MODAL_TOKEN_ID", env.modalID)
	t.Setenv("KAXA_MODAL_TOKEN_SECRET", env.modalSecret)
	t.Setenv("KAXA_DEFAULT_PROVIDER", env.defaultProvider)

	cmd := cli.NewRootCmd(reg)
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs(append([]string{testCmdList}, args...))
	err = cmd.Execute()
	return out.String(), errOut.String(), err
}

func newListFakes() (*listSprites, *listModal) {
	sp := &listSprites{infos: []sandbox.Info{{ID: "s1", Status: "warm", Provider: testSpritesName}}}
	sp.name = testSpritesName
	md := &listModal{infos: []sandbox.Info{{ID: "m1", Status: "running", Provider: testModalName}}}
	md.name = testModalName
	return sp, md
}

var allConfigured = listEnv{spritesToken: listToken, modalID: "ak-1", modalSecret: "as-2"}

func TestListWithoutFlagShowsEveryProvider(t *testing.T) {
	sp, md := newListFakes()
	// The default provider setting must not narrow the list.
	env := allConfigured
	env.defaultProvider = testSpritesName

	out, _, err := runList(t, env, sp, md)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := "name\tstatus\tprovider\tlabels\nm1\trunning\tmodal\t\ns1\twarm\tsprites\t\n"
	if out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

func TestListProviderFlagFilters(t *testing.T) {
	sp, md := newListFakes()

	out, _, err := runList(t, allConfigured, sp, md, testProviderFlag, testModalName)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if strings.Contains(out, "s1") || !strings.Contains(out, "m1") {
		t.Fatalf("output = %q, want only modal", out)
	}
}

func TestListSkipsUnconfiguredProviders(t *testing.T) {
	sp, md := newListFakes()

	out, errOut, err := runList(t, listEnv{spritesToken: listToken}, sp, md)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if strings.Contains(out, "m1") || !strings.Contains(out, "s1") || errOut != "" {
		t.Fatalf("out = %q err = %q", out, errOut)
	}
}

func TestListFlagStillRequiresCredentials(t *testing.T) {
	sp, md := newListFakes()

	_, _, err := runList(t, listEnv{spritesToken: listToken}, sp, md, testProviderFlag, testModalName)
	if !errors.Is(err, sandbox.ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

func TestListWithNoConfiguredProvider(t *testing.T) {
	sp, md := newListFakes()

	_, _, err := runList(t, listEnv{}, sp, md)
	ue, ok := usererr.As(err)
	if !ok || ue.Message != "no provider is configured" {
		t.Fatalf("err = %v", err)
	}
}

func TestListWarnsWhenOneProviderFails(t *testing.T) {
	sp, md := newListFakes()
	md.err = errors.New("boom")

	out, errOut, err := runList(t, allConfigured, sp, md)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "s1") || !strings.Contains(errOut, "could not list modal: boom") {
		t.Fatalf("out = %q err = %q", out, errOut)
	}
}

func TestListFailsWhenEveryProviderFails(t *testing.T) {
	sp, md := newListFakes()
	sp.err, md.err = errors.New("sprites down"), errors.New("modal down")

	_, _, err := runList(t, allConfigured, sp, md)
	if err == nil || !strings.Contains(err.Error(), "sprites down") || !strings.Contains(err.Error(), "modal down") {
		t.Fatalf("err = %v", err)
	}
}

func TestListEmptyAcrossProviders(t *testing.T) {
	sp, md := newListFakes()
	sp.infos, md.infos = nil, nil

	out, _, err := runList(t, allConfigured, sp, md)
	if err != nil || out != "no sandboxes created\n" {
		t.Fatalf("out = %q err = %v", out, err)
	}
}
