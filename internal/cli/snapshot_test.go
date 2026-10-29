package cli_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/cli"
	"github.com/perylemke/kaxa/internal/config"
	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	snapBox     = "box"
	snapOther   = "other"
	snapCmdName = "snapshot"
	snapRestore = "restore"
	snapDelete  = "delete"
)

// snapProvider is a fakeProvider that also saves snapshots.
type snapProvider struct {
	*fakeProvider
	mode      sandbox.RestoreMode
	snaps     []sandbox.Snapshot
	created   []string
	restores  []sandbox.RestoreRequest
	deletes   []sandbox.SnapshotID
	opErr     error
	nextIDNum int
}

func newSnapProvider(name string, mode sandbox.RestoreMode) *snapProvider {
	return &snapProvider{fakeProvider: &fakeProvider{name: name}, mode: mode}
}

func (s *snapProvider) RestoreMode() sandbox.RestoreMode { return s.mode }

func (s *snapProvider) Snapshot(_ context.Context, id sandbox.ID, comment string) (sandbox.Snapshot, error) {
	if s.opErr != nil {
		return sandbox.Snapshot{}, s.opErr
	}
	s.nextIDNum++
	snap := sandbox.Snapshot{
		ID: sandbox.SnapshotID("v" + strconv.Itoa(s.nextIDNum)), Sandbox: id, Provider: s.name,
		Comment: comment, CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	s.created = append(s.created, comment)
	s.snaps = append(s.snaps, snap)
	return snap, nil
}

func (s *snapProvider) ListSnapshots(context.Context, sandbox.ID) ([]sandbox.Snapshot, error) {
	return s.snaps, s.opErr
}

func (s *snapProvider) Restore(_ context.Context, req sandbox.RestoreRequest) (sandbox.ID, error) {
	if s.opErr != nil {
		return "", s.opErr
	}
	s.restores = append(s.restores, req)
	if req.Target != "" {
		return req.Target, nil
	}
	return req.Sandbox, nil
}

func (s *snapProvider) DeleteSnapshot(_ context.Context, _ sandbox.ID, snap sandbox.SnapshotID) error {
	if s.opErr != nil {
		return s.opErr
	}
	s.deletes = append(s.deletes, snap)
	return nil
}

func runSnapshot(t *testing.T, p sandbox.Provider, in string, args ...string) (stdout string, err error) {
	t.Helper()
	cmd := newCreateCmd(t, p)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetIn(strings.NewReader(in))
	cmd.SetArgs(append([]string{testProviderFlag, p.Name()}, args...))
	err = cmd.Execute()
	return out.String(), err
}

func TestSnapshotCreateAndList(t *testing.T) {
	p := newSnapProvider(testSpritesName, sandbox.RestoreInPlace)
	p.list = []sandbox.Info{{ID: snapBox}}

	out, err := runSnapshot(t, p, "", snapCmdName, testCmdCreate, snapBox, "--comment", "before")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for _, want := range []string{"v1", "box on sprites", "created"} {
		if !strings.Contains(out, want) {
			t.Errorf("create output missing %q:\n%s", want, out)
		}
	}
	if len(p.created) != 1 || p.created[0] != "before" {
		t.Fatalf("created = %v", p.created)
	}

	out, err = runSnapshot(t, p, "", snapCmdName, "list", snapBox)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, want := range []string{"PROVIDER", "SANDBOX", "ID", "CREATED", "COMMENT", "sprites", "box", "v1", "2026-01-02T03:04:05Z", "before"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q:\n%s", want, out)
		}
	}
}

func TestSnapshotListEmpty(t *testing.T) {
	out, err := runSnapshot(t, newSnapProvider(testSpritesName, sandbox.RestoreInPlace), "", snapCmdName, "list", snapBox)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "no snapshots for box") {
		t.Fatalf("output = %q", out)
	}
}

func TestSnapshotRestoreInPlace(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		args     []string
		wantRun  bool
		wantText string
	}{
		{"confirmed", "y\n", nil, true, "restored to snapshot v1"},
		{"declined", "n\n", nil, false, "cancelled"},
		{"force", "", []string{"--force"}, true, "restored to snapshot v1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newSnapProvider(testSpritesName, sandbox.RestoreInPlace)
			args := append([]string{snapCmdName, snapRestore, snapBox, "v1"}, tt.args...)
			out, err := runSnapshot(t, p, tt.in, args...)
			if err != nil {
				t.Fatalf("restore: %v", err)
			}
			if (len(p.restores) == 1) != tt.wantRun {
				t.Fatalf("restores = %+v, wantRun %v", p.restores, tt.wantRun)
			}
			if !strings.Contains(out, tt.wantText) {
				t.Fatalf("output missing %q:\n%s", tt.wantText, out)
			}
		})
	}
}

func TestSnapshotRestoreNewSandbox(t *testing.T) {
	p := newSnapProvider(snapOther, sandbox.RestoreNewSandbox)

	out, err := runSnapshot(t, p, "", snapCmdName, snapRestore, snapBox, "im-1", "--to", "copy")
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	want := sandbox.RestoreRequest{Sandbox: snapBox, Snapshot: "im-1", Target: "copy"}
	if len(p.restores) != 1 || p.restores[0] != want {
		t.Fatalf("restores = %+v", p.restores)
	}
	if !strings.Contains(out, "copy on other") || !strings.Contains(out, "source was kept") {
		t.Fatalf("output = %q", out)
	}

	_, err = runSnapshot(t, p, "", snapCmdName, snapRestore, snapBox, "im-1")
	if _, ok := usererr.As(err); !ok {
		t.Fatalf("missing --to err = %v, want a user error", err)
	}
	if len(p.restores) != 1 {
		t.Fatal("restore ran without a target")
	}
}

func TestSnapshotRestoreInPlaceRejectsTarget(t *testing.T) {
	p := newSnapProvider(testSpritesName, sandbox.RestoreInPlace)
	_, err := runSnapshot(t, p, "y\n", snapCmdName, snapRestore, snapBox, "v1", "--to", "copy")
	if _, ok := usererr.As(err); !ok {
		t.Fatalf("err = %v, want a user error", err)
	}
	if len(p.restores) != 0 {
		t.Fatal("restore ran with a target on an in-place provider")
	}
}

func TestSnapshotDelete(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		args    []string
		wantRun bool
	}{
		{"confirmed", "y\n", nil, true},
		{"declined", "n\n", nil, false},
		{"force", "", []string{"-f"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newSnapProvider(testSpritesName, sandbox.RestoreInPlace)
			args := append([]string{snapCmdName, snapDelete, snapBox, "v1"}, tt.args...)
			if _, err := runSnapshot(t, p, tt.in, args...); err != nil {
				t.Fatalf("delete: %v", err)
			}
			if (len(p.deletes) == 1) != tt.wantRun {
				t.Fatalf("deletes = %v, wantRun %v", p.deletes, tt.wantRun)
			}
		})
	}
}

func TestSnapshotArgumentErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"create without name", []string{snapCmdName, testCmdCreate}},
		{"create with extra", []string{snapCmdName, testCmdCreate, "a", "b"}},
		{"list with extra", []string{snapCmdName, testCmdList, "a", "b"}},
		{"restore without id", []string{snapCmdName, snapRestore, snapBox}},
		{"delete without id", []string{snapCmdName, snapDelete, snapBox}},
		{"delete bad id", []string{snapCmdName, snapDelete, snapBox, "../x", "-f"}},
		{"restore bad id", []string{snapCmdName, snapRestore, snapBox, "a/b", "-f"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newSnapProvider(testSpritesName, sandbox.RestoreInPlace)
			_, err := runSnapshot(t, p, "", tt.args...)
			if _, ok := usererr.As(err); !ok {
				t.Fatalf("err = %v, want a user error", err)
			}
			if len(p.restores)+len(p.deletes)+len(p.created) != 0 {
				t.Fatal("a snapshot operation ran despite the bad arguments")
			}
		})
	}
}

func TestSnapshotProviderErrorIsReturned(t *testing.T) {
	p := newSnapProvider(testSpritesName, sandbox.RestoreInPlace)
	p.opErr = errors.New("boom")
	if _, err := runSnapshot(t, p, "", snapCmdName, testCmdCreate, snapBox); !errors.Is(err, p.opErr) {
		t.Fatalf("err = %v, want boom", err)
	}
}

func TestSnapshotRejectedWithoutCapability(t *testing.T) {
	for _, sub := range [][]string{
		{testCmdCreate, snapBox},
		{"list", snapBox},
		{snapRestore, snapBox, "v1"},
		{snapDelete, snapBox, "v1"},
	} {
		t.Run(sub[0], func(t *testing.T) {
			_, err := runSnapshot(t, coreProvider{name: plainName}, "", append([]string{snapCmdName}, sub...)...)
			ue, ok := errors.AsType[*sandbox.UnsupportedError](err)
			if !errors.Is(err, sandbox.ErrUnsupported) || !ok || ue.Feature != sandbox.FeatureSnapshot {
				t.Fatalf("err = %v, want unsupported snapshot", err)
			}
		})
	}
}

func TestSnapshotUnsupportedHintNamesAlternatives(t *testing.T) {
	reg := registry.New()
	for _, p := range []sandbox.Provider{newSnapProvider(testSpritesName, sandbox.RestoreInPlace), coreProvider{name: plainName}} {
		if err := reg.Register(p); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	t.Setenv("KAXA_SPRITES_TOKEN", "tok")
	cmd := cli.NewRootCmd(reg)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{testProviderFlag, plainName, snapCmdName, "list", snapBox})

	// cli.Execute applies WithAlternatives to the command error before printing it.
	err := lifecycle.WithAlternatives(reg, config.Config{Sprites: config.SpritesConfig{Token: "tok"}}, cmd.Execute())
	ue, ok := usererr.As(err)
	if !ok || ue.Message != "snapshot is not available for plain" || !strings.Contains(ue.Hint, testSpritesName) {
		t.Fatalf("err = %#v", err)
	}
}

// snapModal is a snapshot provider that Modal's credentials configure.
type snapModal struct{ *snapProvider }

func (snapModal) ConfigureCredentials(id, secret string) error {
	if id == "" || secret == "" {
		return usererr.Wrap(sandbox.ErrNotConfigured, "missing", "")
	}
	return nil
}

// runSnapshotAll runs kaxa with a Sprites and a Modal provider, both configured.
func runSnapshotAll(t *testing.T, sp, md *snapProvider, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	reg := registry.New()
	for _, p := range []sandbox.Provider{sp, snapModal{md}} {
		if regErr := reg.Register(p); regErr != nil {
			t.Fatalf("register: %v", regErr)
		}
	}
	t.Setenv("KAXA_SPRITES_TOKEN", listToken)
	t.Setenv("KAXA_MODAL_TOKEN_ID", "ak-id")
	t.Setenv("KAXA_MODAL_TOKEN_SECRET", "as-secret")
	t.Setenv("KAXA_DEFAULT_PROVIDER", "")

	cmd := cli.NewRootCmd(reg)
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs(append([]string{snapCmdName, "list"}, args...))
	err = cmd.Execute()
	return out.String(), errOut.String(), err
}

func newSnapshotPair() (sp, md *snapProvider) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	sp = newSnapProvider(testSpritesName, sandbox.RestoreInPlace)
	sp.list = []sandbox.Info{{ID: "web"}}
	sp.snaps = []sandbox.Snapshot{{ID: "v2", Comment: "first", CreatedAt: at}}
	md = newSnapProvider(testModalName, sandbox.RestoreNewSandbox)
	md.list = []sandbox.Info{{ID: "api"}}
	md.snaps = []sandbox.Snapshot{{ID: "im-1", Comment: "second", CreatedAt: at.Add(time.Hour)}}
	return sp, md
}

func TestSnapshotListAllProviders(t *testing.T) {
	sp, md := newSnapshotPair()
	out, errOut, err := runSnapshotAll(t, sp, md)
	if err != nil || errOut != "" {
		t.Fatalf("err = %v, stderr = %q", err, errOut)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("output has %d lines, want a header and two rows:\n%s", len(lines), out)
	}
	for i, want := range [][]string{
		{"PROVIDER", "SANDBOX", "ID", "CREATED", "COMMENT"},
		{testModalName, "api", "im-1", "2026-01-02T04:04:05Z", "second"}, // registry order: modal, sprites
		{testSpritesName, "web", "v2", "2026-01-02T03:04:05Z", "first"},
	} {
		if got := strings.Fields(lines[i]); !slices.Equal(got, want) {
			t.Errorf("line %d = %v, want %v", i, got, want)
		}
	}
}

func TestSnapshotListNamedSandboxSearchesEveryProvider(t *testing.T) {
	sp, md := newSnapshotPair()
	out, _, err := runSnapshotAll(t, sp, md, "api")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "im-1") || strings.Contains(out, "v2") {
		t.Fatalf("output = %q, want only the snapshots of api", out)
	}
}

func TestSnapshotListProviderFlagNarrows(t *testing.T) {
	sp, md := newSnapshotPair()
	out, _, err := runSnapshotAll(t, sp, md, testProviderFlag, testModalName)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "im-1") || strings.Contains(out, "v2") {
		t.Fatalf("output = %q, want only modal", out)
	}
}

func TestSnapshotListWarnsWhenAProviderFails(t *testing.T) {
	sp, md := newSnapshotPair()
	sp.opErr = errors.New("boom")
	out, errOut, err := runSnapshotAll(t, sp, md)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "im-1") {
		t.Fatalf("output = %q, want the provider that works", out)
	}
	if !strings.Contains(errOut, "warning: could not list snapshots on sprites") || !strings.Contains(errOut, "boom") {
		t.Fatalf("stderr = %q", errOut)
	}
}

func TestSnapshotListEverythingEmpty(t *testing.T) {
	sp, md := newSnapshotPair()
	sp.snaps, md.snaps = nil, nil
	out, _, err := runSnapshotAll(t, sp, md)
	if err != nil || strings.TrimSpace(out) != "no snapshots" {
		t.Fatalf("output = %q, err = %v", out, err)
	}
}
