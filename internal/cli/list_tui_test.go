package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestListInteractiveDetailViewSprites(t *testing.T) {
	t.Parallel()

	now := time.Now()
	m := listInteractiveModel{
		mode: listModeDetail,
		providers: map[string]sandbox.Provider{
			spritesProviderName: &recordingURLProvider{},
		},
		infos: []sandbox.Info{{ID: fixtureDemo, Provider: spritesProviderName}},
		detail: sandbox.Detail{
			ID:       fixtureDemo,
			Provider: spritesProviderName,
			Status:   fixtureWarm,
			Labels:   []string{"kaxa", "team=a"},
			Fields: []sandbox.Field{
				{Label: "ID", Text: "sprite-uuid-1"},
				{Label: "Region", Text: "iad"},
				{Label: "CPUs", Text: "2"},
				{Label: "Last running", At: now.Add(-time.Hour)},
				{Label: "URL", Text: "https://demo.sprites.dev"},
				{Label: "Auth", Text: "sprite · admins"},
			},
		},
	}
	view := stripANSI(m.View())
	for _, want := range []string{
		fixtureDemo,
		spritesProviderName,
		fixtureWarm,
		"sprite-uuid-1",
		"iad",
		"CPUs",
		"Last running",
		"https://demo.sprites.dev",
		"sprite · admins",
		"kaxa,team=a",
		"r restart",
		"enter auth",
		"u upgrade",
		"c console",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestListInteractiveDetailViewModal(t *testing.T) {
	t.Parallel()

	m := listInteractiveModel{
		mode: listModeDetail,
		providers: map[string]sandbox.Provider{
			formModalProvider: modalListStub{},
		},
		infos: []sandbox.Info{{ID: fixtureDemo, Provider: formModalProvider}},
		detail: sandbox.Detail{
			ID:       fixtureDemo,
			Provider: formModalProvider,
			Status:   "running",
			Fields:   []sandbox.Field{{Label: "ID", Text: "sb-abc"}},
		},
	}
	view := stripANSI(m.View())
	for _, mustNot := range []string{"Updated", "Organization", "enter auth", "r restart", "u upgrade"} {
		if strings.Contains(view, mustNot) {
			t.Fatalf("view must not contain %q:\n%s", mustNot, view)
		}
	}
	if !strings.Contains(view, "sb-abc") || !strings.Contains(view, "c console") || !strings.Contains(view, "d destroy") {
		t.Fatalf("view = %q", view)
	}
}

func TestListInteractiveDetailViewE2B(t *testing.T) {
	t.Parallel()

	m := listInteractiveModel{
		mode: listModeDetail,
		providers: map[string]sandbox.Provider{
			e2bProviderName: e2bListStub{},
		},
		infos:  []sandbox.Info{{ID: fixtureDemo, Provider: e2bProviderName}},
		detail: sandbox.Detail{ID: fixtureDemo, Provider: e2bProviderName, Status: "running"},
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "r restart") {
		t.Fatalf("view = %q", view)
	}
	for _, mustNot := range []string{"enter auth", "u upgrade"} {
		if strings.Contains(view, mustNot) {
			t.Fatalf("view must not contain %q:\n%s", mustNot, view)
		}
	}
}

func TestListInteractiveListViewShowsHelp(t *testing.T) {
	t.Parallel()

	m := newListInteractiveModel(context.TODO(), nil, []sandbox.Info{
		{ID: "a", Status: fixtureWarm, Provider: spritesProviderName},
	}, 0)
	m.mode = listModeTable
	view := stripANSI(m.View())
	if !strings.Contains(view, "enter detail") || !strings.Contains(view, "a") {
		t.Fatalf("view = %q", view)
	}
}

func TestListInteractiveApplyDetailError(t *testing.T) {
	t.Parallel()

	m := listInteractiveModel{mode: listModeTable}
	next := m.applyDetail(detailLoadedMsg{err: errors.New("boom")})
	if next.mode != listModeDetail || next.detailErr == nil {
		t.Fatalf("mode=%v err=%v", next.mode, next.detailErr)
	}
}

func TestListInteractiveUpdateDetailLoadedMsg(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	providers := map[string]sandbox.Provider{spritesProviderName: idleProvider{}}
	infos := []sandbox.Info{{ID: "a", Status: fixtureWarm, Provider: spritesProviderName}}
	m := newListInteractiveModel(ctx, providers, infos, 0)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected inspect cmd")
	}
	lm := authModel(t, next)
	if lm.mode != listModeDetailLoading {
		t.Fatalf("mode = %d", lm.mode)
	}
	msg := cmd()
	next2, _ := lm.Update(msg)
	got := authModel(t, next2)
	if got.mode != listModeDetail {
		t.Fatalf("mode = %d", got.mode)
	}
}

func TestListInteractiveUpdateIgnoresUnknownMsg(t *testing.T) {
	t.Parallel()

	m := newListInteractiveModel(context.Background(), nil, []sandbox.Info{
		{ID: "a", Status: fixtureWarm, Provider: spritesProviderName},
	}, 0)
	next, cmd := m.Update(struct{ x int }{1})
	if cmd != nil {
		t.Fatalf("cmd = %v", cmd)
	}
	authModel(t, next)
}

func TestListInteractiveTableEnterNoSandboxes(t *testing.T) {
	t.Parallel()

	m := newListInteractiveModel(context.Background(), nil, []sandbox.Info{}, 0)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("expected no cmd")
	}
	authModel(t, next)
}

func TestListInteractiveDetailLoadingQuit(t *testing.T) {
	t.Parallel()

	m := listInteractiveModel{mode: listModeDetailLoading}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected quit")
	}
	_ = authModel(t, next)
}

func TestInspectCmdNilProvider(t *testing.T) {
	t.Parallel()

	cmd := inspectCmd(context.Background(), nil, sandbox.Info{Provider: "sprites", ID: "x"})
	msg := cmd()
	d, ok := msg.(detailLoadedMsg)
	if !ok || d.err == nil {
		t.Fatalf("msg = %#v", msg)
	}
}

func TestHandleListPostActionStreamsMissingProvider(t *testing.T) {
	t.Parallel()

	err := handleListPostActionStreams(
		context.Background(),
		strings.NewReader("y\n"),
		new(bytes.Buffer),
		new(bytes.Buffer),
		map[string]sandbox.Provider{},
		listTUIResult{postAction: listPostRestart, id: fixtureDemo, provider: spritesProviderName},
	)
	if err == nil || !strings.Contains(err.Error(), "is not configured") {
		t.Fatalf("err = %v", err)
	}
}

func TestListTableHeightCapsAtMax(t *testing.T) {
	t.Parallel()

	if got := listTableHeight(100); got != 20 {
		t.Fatalf("got %d", got)
	}
}

func TestListInfoIndexMissing(t *testing.T) {
	t.Parallel()

	if got := listInfoIndex([]sandbox.Info{{ID: "a", Provider: spritesProviderName}}, "sprites", "b"); got != -1 {
		t.Fatalf("got %d", got)
	}
}

func TestDetailIDUsesRowWhenDetailEmpty(t *testing.T) {
	t.Parallel()

	m := listInteractiveModel{
		mode:      listModeDetail,
		infos:     []sandbox.Info{{ID: "row-id", Provider: spritesProviderName}},
		detail:    sandbox.Detail{},
		detailErr: lifecycle.Unsupported(&idleProvider{}, sandbox.FeatureRestart),
	}
	m.table = newListTable(m.infos, true, listTableHeight(len(m.infos)))
	if got := m.detailID(); got != "row-id" {
		t.Fatalf("detailID = %q", got)
	}
}

func TestFormatRelativeTimeNonZero(t *testing.T) {
	t.Parallel()

	s := formatRelativeTime(time.Now().Add(-2 * time.Second))
	if !strings.Contains(s, "ago") {
		t.Fatalf("got %q", s)
	}
}

func TestListInteractiveUpdateKeyDefaultBranch(t *testing.T) {
	t.Parallel()

	m := listInteractiveModel{mode: 99}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	am := authModel(t, next)
	if cmd != nil || am.mode != 99 {
		t.Fatalf("cmd=%v mode=%d", cmd, am.mode)
	}
}

func TestDetailFeatureActionNilProvider(t *testing.T) {
	t.Parallel()

	m := listInteractiveModel{mode: listModeDetail, detail: sandbox.Detail{ID: "x"}}
	next, cmd := m.detailFeatureAction(nil, sandbox.FeatureRestart, listPostRestart, "x", spritesProviderName)
	got := authModel(t, next)
	if cmd != nil || got.postAction != listPostNone || got.detailMsg != "" {
		t.Fatalf("postAction=%v msg=%q", got.postAction, got.detailMsg)
	}
}

func TestShowUnsupportedNilProvider(t *testing.T) {
	t.Parallel()

	m := listInteractiveModel{}
	out := m.showUnsupported(nil, sandbox.FeatureRestart)
	if out.detailMsg != "" {
		t.Fatalf("msg = %q", out.detailMsg)
	}
}

func TestListEnterOpensURLAuth(t *testing.T) {
	t.Parallel()

	p := &recordingURLProvider{}
	m := listInteractiveModel{
		mode: listModeDetail,
		providers: map[string]sandbox.Provider{
			spritesProviderName: p,
		},
		infos:  []sandbox.Info{{ID: fixtureDemo, Provider: spritesProviderName}},
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

func TestListDetailUnsupportedKeyStaysOnScreen(t *testing.T) {
	t.Parallel()

	m := listInteractiveModel{
		mode: listModeDetail,
		providers: map[string]sandbox.Provider{
			formModalProvider: modalListStub{},
		},
		infos:  []sandbox.Info{{ID: fixtureDemo, Provider: formModalProvider}},
		detail: sandbox.Detail{ID: fixtureDemo, Provider: formModalProvider},
	}
	next, cmd := m.updateDetailKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := authModel(t, next)
	if got.postAction != listPostNone || cmd != nil {
		t.Fatalf("action=%v cmd=%v", got.postAction, cmd)
	}
	view := stripANSI(got.View())
	if !strings.Contains(view, "url auth is not available for modal") {
		t.Fatalf("view = %q", view)
	}
}

func TestListDetailUnsupportedRestartMessage(t *testing.T) {
	t.Parallel()

	m := listInteractiveModel{
		mode: listModeDetail,
		providers: map[string]sandbox.Provider{
			formModalProvider: modalListStub{},
		},
		infos:  []sandbox.Info{{ID: fixtureDemo, Provider: formModalProvider}},
		detail: sandbox.Detail{ID: fixtureDemo, Provider: formModalProvider},
	}
	next, cmd := m.updateDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	got := authModel(t, next)
	if got.postAction != listPostNone || cmd != nil {
		t.Fatalf("action=%v", got.postAction)
	}
	if !strings.Contains(stripANSI(got.View()), "restart is not available for modal") {
		t.Fatalf("view = %q", stripANSI(got.View()))
	}
}

func TestListDetailConsoleKey(t *testing.T) {
	t.Parallel()

	m := listInteractiveModel{
		mode: listModeDetail,
		providers: map[string]sandbox.Provider{
			formModalProvider: modalListStub{},
		},
		infos:  []sandbox.Info{{ID: fixtureDemo, Provider: formModalProvider}},
		detail: sandbox.Detail{ID: fixtureDemo, Provider: formModalProvider},
	}
	next, cmd := m.updateDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	got := authModel(t, next)
	if got.postAction != listPostConsole || got.postID != fixtureDemo || got.postProv != formModalProvider {
		t.Fatalf("action=%v id=%q provider=%q", got.postAction, got.postID, got.postProv)
	}
	if cmd == nil {
		t.Fatal("expected quit")
	}
}

func TestListConsoleLoopReopensTable(t *testing.T) {
	t.Parallel()

	infos := []sandbox.Info{
		{ID: "a", Provider: spritesProviderName},
		{ID: fixtureDemo, Provider: formModalProvider},
	}
	providers := map[string]sandbox.Provider{
		spritesProviderName: &recordingURLProvider{},
		formModalProvider:   modalListStub{},
	}
	calls := 0
	run := func(_ context.Context, _ map[string]sandbox.Provider, _ []sandbox.Info, startCursor int) (listTUIResult, error) {
		calls++
		if calls == 1 {
			if startCursor != 0 {
				t.Fatalf("start cursor = %d", startCursor)
			}
			return listTUIResult{postAction: listPostConsole, id: fixtureDemo, provider: formModalProvider}, nil
		}
		if startCursor != 1 {
			t.Fatalf("second start cursor = %d, want 1", startCursor)
		}
		return listTUIResult{}, nil
	}
	opened := false
	open := func(_ context.Context, p sandbox.Provider, id sandbox.ID, _, _ io.Writer) error {
		if p.Name() != formModalProvider || id != fixtureDemo {
			t.Fatalf("open %q %q", p.Name(), id)
		}
		opened = true
		return nil
	}
	err := runListWithConsoleLoop(context.Background(), providers, infos, strings.NewReader(""), new(bytes.Buffer), new(bytes.Buffer), run, open)
	if err != nil {
		t.Fatal(err)
	}
	if !opened || calls != 2 {
		t.Fatalf("opened=%v calls=%d", opened, calls)
	}
}

func TestListConsoleLoopOpenErrorContinues(t *testing.T) {
	t.Parallel()

	infos := []sandbox.Info{{ID: fixtureDemo, Provider: formModalProvider}}
	providers := map[string]sandbox.Provider{formModalProvider: modalListStub{}}
	calls := 0
	run := func(_ context.Context, _ map[string]sandbox.Provider, _ []sandbox.Info, _ int) (listTUIResult, error) {
		calls++
		if calls == 1 {
			return listTUIResult{postAction: listPostConsole, id: fixtureDemo, provider: formModalProvider}, nil
		}
		return listTUIResult{}, nil
	}
	open := func(context.Context, sandbox.Provider, sandbox.ID, io.Writer, io.Writer) error {
		return errors.New("console failed")
	}
	errOut := new(bytes.Buffer)
	err := runListWithConsoleLoop(context.Background(), providers, infos, strings.NewReader(""), new(bytes.Buffer), errOut, run, open)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !strings.Contains(errOut.String(), "console failed") {
		t.Fatalf("calls=%d stderr=%q", calls, errOut.String())
	}
}

func TestListDetailEscClearsUnsupportedMessage(t *testing.T) {
	t.Parallel()

	m := listInteractiveModel{
		mode:      listModeDetail,
		providers: map[string]sandbox.Provider{formModalProvider: modalListStub{}},
		detailMsg: "restart is not available for modal",
	}
	next, _ := m.updateDetailKey(tea.KeyMsg{Type: tea.KeyEsc})
	got := authModel(t, next)
	if got.mode != listModeTable || got.detailMsg != "" {
		t.Fatalf("mode=%v msg=%q", got.mode, got.detailMsg)
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

	err := runListURLAuth(context.Background(), strings.NewReader("y\n"), new(bytes.Buffer), new(bytes.Buffer), modalListStub{}, listTUIResult{
		id:   fixtureDemo,
		auth: sandbox.URLAuthPublic,
	})
	if err == nil || !strings.Contains(err.Error(), "url auth is not available for") {
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

type modalListStub struct{}

func (modalListStub) Name() string { return formModalProvider }

func (modalListStub) Create(context.Context, sandbox.Spec) (sandbox.ID, error) { return "", nil }

func (modalListStub) List(context.Context) ([]sandbox.Info, error) { return nil, nil }

func (modalListStub) Inspect(context.Context, sandbox.ID) (sandbox.Detail, error) {
	return sandbox.Detail{}, nil
}

func (modalListStub) Exec(context.Context, sandbox.ID, sandbox.ExecOptions) error { return nil }

func (modalListStub) Destroy(context.Context, sandbox.ID) error { return nil }

type e2bListStub struct{}

func (e2bListStub) Name() string { return e2bProviderName }

func (e2bListStub) Create(context.Context, sandbox.Spec) (sandbox.ID, error) { return "", nil }

func (e2bListStub) List(context.Context) ([]sandbox.Info, error) { return nil, nil }

func (e2bListStub) Inspect(context.Context, sandbox.ID) (sandbox.Detail, error) {
	return sandbox.Detail{}, nil
}

func (e2bListStub) Restart(context.Context, sandbox.ID) error { return nil }

func (e2bListStub) Exec(context.Context, sandbox.ID, sandbox.ExecOptions) error { return nil }

func (e2bListStub) Destroy(context.Context, sandbox.ID) error { return nil }

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

func TestListDetailActionKeepsRowProvider(t *testing.T) {
	t.Parallel()

	m := newListInteractiveModel(context.TODO(), nil, []sandbox.Info{
		{ID: "a", Status: fixtureWarm, Provider: spritesProviderName},
	}, 0)
	m.mode = listModeDetail
	m.detail = sandbox.Detail{ID: "a"}

	next, _ := m.updateDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	got := authModel(t, next)
	if got.postAction != listPostDestroy || got.postID != "a" || got.postProv != spritesProviderName {
		t.Fatalf("action=%v id=%q provider=%q", got.postAction, got.postID, got.postProv)
	}
}
