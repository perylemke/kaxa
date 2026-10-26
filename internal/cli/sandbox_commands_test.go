package cli_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/perylemke/sbox/internal/cli"
	"github.com/perylemke/sbox/internal/provider/sprites"
	"github.com/perylemke/sbox/internal/registry"
	"github.com/perylemke/sbox/internal/sandbox"
	"github.com/perylemke/sbox/internal/usererr"
)

const (
	testProviderFlag = "--provider"
	testSpritesName  = "sprites"
	testRunningFlag  = "--running"
)

type fakeProvider struct {
	name           string
	configureToken string
	createID       sandbox.ID
	lastCreate     sandbox.Spec
	list           []sandbox.Info
	detail         sandbox.Detail
	inspectErr     error
	restarted      sandbox.ID
	restartErr     error
	upgraded       sandbox.ID
	upgradeErr     error
	lastExec       sandbox.ExecOptions
	destroyed      sandbox.ID
	destroyedIDs   []sandbox.ID
	destroyErr     error
	execErr        error
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Configure(token string) error {
	f.configureToken = token
	if token == "" {
		return sprites.ErrMissingToken
	}
	return nil
}

func (f *fakeProvider) Create(_ context.Context, spec sandbox.Spec) (sandbox.ID, error) {
	f.lastCreate = spec
	if f.createID != "" {
		return f.createID, nil
	}
	return sandbox.ID(spec.Name), nil
}

func (f *fakeProvider) List(context.Context) ([]sandbox.Info, error) {
	return f.list, nil
}

func (f *fakeProvider) Inspect(_ context.Context, id sandbox.ID) (sandbox.Detail, error) {
	if f.inspectErr != nil {
		return sandbox.Detail{}, f.inspectErr
	}
	if f.detail.ID != "" {
		return f.detail, nil
	}
	return sandbox.Detail{ID: id, Provider: f.name, Status: testStatusWarm}, nil
}

func (f *fakeProvider) Restart(_ context.Context, id sandbox.ID) error {
	f.restarted = id
	return f.restartErr
}

func (f *fakeProvider) Upgrade(_ context.Context, id sandbox.ID) error {
	f.upgraded = id
	return f.upgradeErr
}

func (f *fakeProvider) Exec(_ context.Context, _ sandbox.ID, opts sandbox.ExecOptions) error {
	f.lastExec = opts
	return f.execErr
}

func (f *fakeProvider) Destroy(_ context.Context, id sandbox.ID) error {
	f.destroyed = id
	f.destroyedIDs = append(f.destroyedIDs, id)
	return f.destroyErr
}

func spritesRegistry(t *testing.T, p sandbox.Provider) *registry.Registry {
	t.Helper()
	reg := registry.New()
	if err := reg.Register(p); err != nil {
		t.Fatalf("register: %v", err)
	}
	return reg
}

func TestCreatePrintsName(t *testing.T) {
	fp := &fakeProvider{name: testSpritesName, createID: testBoxName}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdCreate, testBoxName})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "created") || !strings.Contains(got, testBoxName) || !strings.Contains(got, "ready") {
		t.Fatalf("output = %q", got)
	}
	if fp.configureToken != "tok" {
		t.Fatalf("token = %q", fp.configureToken)
	}
	if fp.lastCreate.Labels == nil || fp.lastCreate.Labels[0] != "sbox" {
		t.Fatalf("create labels = %v", fp.lastCreate.Labels)
	}
}

func TestCreateInteractiveForm(t *testing.T) {
	fp := &fakeProvider{name: testSpritesName, createID: testBoxName}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetIn(strings.NewReader(strings.Join([]string{"demo", "1", "y", "n", "n", "n", ""}, "\n")))
	cmd.SetArgs([]string{testCmdCreate})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if fp.lastCreate.Name != "demo" || !fp.lastCreate.Running {
		t.Fatalf("create = %+v", fp.lastCreate)
	}
	if fp.configureToken != "tok" {
		t.Fatalf("token = %q", fp.configureToken)
	}
}

func TestCreateRunningSetsSpec(t *testing.T) {
	restore := cli.SetOpenBrowserForTest(nil)
	t.Cleanup(restore)

	const spriteURL = "https://my-box-bpcxj.sprites.app"
	fp := &fakeProvider{
		name:     testSpritesName,
		createID: testBoxName,
		detail:   sandbox.Detail{ID: testBoxName, URL: spriteURL},
	}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdCreate, testBoxName, testRunningFlag})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !fp.lastCreate.Running {
		t.Fatalf("Running = false, want true")
	}
	got := out.String()
	if !strings.Contains(got, "stay running for 1 hour") || !strings.Contains(got, spriteURL) {
		t.Fatalf("output = %q", got)
	}
}

func TestCreateWithoutRunningFlag(t *testing.T) {
	fp := &fakeProvider{name: testSpritesName, createID: testBoxName}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdCreate, testBoxName})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if fp.lastCreate.Running {
		t.Fatal("Running should be false by default")
	}
	if strings.Contains(out.String(), "stay running") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestCreateAliasNew(t *testing.T) {
	fp := &fakeProvider{name: testSpritesName, createID: testBoxName}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, "new", testBoxName})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if fp.lastCreate.Name != testBoxName {
		t.Fatalf("created = %q", fp.lastCreate.Name)
	}
}

func TestListAliasLs(t *testing.T) {
	fp := &fakeProvider{name: testSpritesName}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, "ls"})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "no sandboxes created") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestCreateNoSpinnerOnNonTTYStderr(t *testing.T) {
	fp := &fakeProvider{name: testSpritesName, createID: testBoxName}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdCreate, testBoxName})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q, want empty without TTY", errOut.String())
	}
}

func TestListFormatsOutput(t *testing.T) {
	fp := &fakeProvider{
		name: testSpritesName,
		list: []sandbox.Info{
			{ID: "a", Status: testStatusCold, Provider: testSpritesName},
			{ID: "b", Status: testStatusWarm, Provider: testSpritesName},
		},
	}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdList})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	want := "name\tstatus\tprovider\tlabels\na\tcold\tsprites\t\nb\twarm\tsprites\t\n"
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func TestListEmptyPrintsMessage(t *testing.T) {
	fp := &fakeProvider{name: testSpritesName}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdList})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.String() != "no sandboxes created\n" {
		t.Fatalf("output = %q", out.String())
	}
}

func runSpritesCmd(t *testing.T, args ...string) (*fakeProvider, error) {
	t.Helper()
	fp := &fakeProvider{name: testSpritesName}
	cmd := cli.NewRootCmd(spritesRegistry(t, fp))
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs(append([]string{testProviderFlag, testSpritesName}, args...))
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")
	return fp, cmd.Execute()
}

func TestExecRequiresCommand(t *testing.T) {
	fp, err := runSpritesCmd(t, testCmdExec, testArgBox)
	if err == nil || !strings.Contains(err.Error(), "exec requires a command") {
		t.Fatalf("err = %v", err)
	}
	if len(fp.lastExec.Command) != 0 {
		t.Fatalf("exec called: %v", fp.lastExec.Command)
	}
}

func TestConsoleRequiresName(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing", args: []string{testCmdConsole}, want: "console requires a sandbox name"},
		{name: "ssh missing", args: []string{"ssh"}, want: "console requires a sandbox name"},
		{name: "extra", args: []string{testCmdConsole, testArgBox, "extra"}, want: "console accepts one sandbox name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fp, err := runSpritesCmd(t, tt.args...)
			ue, ok := usererr.As(err)
			if !ok || ue.Message != tt.want || ue.Hint != "sbox console <name>" {
				t.Fatalf("err = %#v", err)
			}
			if len(fp.lastExec.Command) != 0 {
				t.Fatalf("exec called: %v", fp.lastExec.Command)
			}
		})
	}
}

func TestConsoleRequiresTerminal(t *testing.T) {
	for _, sub := range []string{testCmdConsole, "ssh"} {
		t.Run(sub, func(t *testing.T) {
			fp, err := runSpritesCmd(t, sub, testArgBox)
			if err == nil || !strings.Contains(err.Error(), "interactive shell requires a terminal") {
				t.Fatalf("err = %v", err)
			}
			if len(fp.lastExec.Command) != 0 {
				t.Fatalf("exec called: %v", fp.lastExec.Command)
			}
		})
	}
}

func TestExecWithArgsNoTTY(t *testing.T) {
	fp := &fakeProvider{name: testSpritesName}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdExec, testArgBox, "--", "ls", "-la"})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if fp.lastExec.TTY {
		t.Fatal("expected non-interactive exec")
	}
	if len(fp.lastExec.Command) != 2 || fp.lastExec.Command[0] != "ls" {
		t.Fatalf("command = %v", fp.lastExec.Command)
	}
}

func TestDestroyCommands(t *testing.T) {
	const sandboxName = "gone"

	for _, sub := range []string{testCmdDestroy, "rm"} {
		t.Run(sub, func(t *testing.T) {
			fp := &fakeProvider{name: testSpritesName}
			reg := spritesRegistry(t, fp)
			cmd := cli.NewRootCmd(reg)
			out := new(bytes.Buffer)
			cmd.SetOut(out)
			cmd.SetErr(new(bytes.Buffer))
			cmd.SetIn(strings.NewReader("y\n"))
			cmd.SetArgs([]string{testProviderFlag, testSpritesName, sub, sandboxName})
			t.Setenv("SBOX_SPRITES_TOKEN", "tok")

			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if fp.destroyed != sandboxName {
				t.Fatalf("destroyed = %q", fp.destroyed)
			}
			if !strings.Contains(out.String(), "destroyed") || !strings.Contains(out.String(), sandboxName) {
				t.Fatalf("output = %q", out.String())
			}
		})
	}
}

func TestDestroyForceSkipsConfirm(t *testing.T) {
	for _, flag := range []string{"--force", "-f"} {
		t.Run(flag, func(t *testing.T) {
			fp := &fakeProvider{name: testSpritesName}
			reg := spritesRegistry(t, fp)
			cmd := cli.NewRootCmd(reg)
			out := new(bytes.Buffer)
			errOut := new(bytes.Buffer)
			cmd.SetOut(out)
			cmd.SetErr(errOut)
			cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdDestroy, flag, testBoxName})
			t.Setenv("SBOX_SPRITES_TOKEN", "tok")

			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if fp.destroyed != testBoxName {
				t.Fatalf("destroyed = %q", fp.destroyed)
			}
			if strings.Contains(errOut.String(), "[y/N]") {
				t.Fatalf("prompt = %q", errOut.String())
			}
			if !strings.Contains(out.String(), "sandbox "+testBoxName+" was destroyed") {
				t.Fatalf("output = %q", out.String())
			}
		})
	}
}

func TestDestroyAllForce(t *testing.T) {
	fp := &fakeProvider{
		name: testSpritesName,
		list: []sandbox.Info{{ID: "a"}, {ID: "b"}},
	}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdDestroy, "-f", testFlagAll})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(fp.destroyedIDs) != 2 || fp.destroyedIDs[0] != "a" || fp.destroyedIDs[1] != "b" {
		t.Fatalf("destroyed = %v", fp.destroyedIDs)
	}
	if strings.Contains(errOut.String(), "[y/N]") {
		t.Fatalf("prompt = %q", errOut.String())
	}
	if !strings.Contains(out.String(), "2 sandboxes were destroyed") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestDestroyNoSkipsProvider(t *testing.T) {
	fp := &fakeProvider{name: testSpritesName}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetIn(strings.NewReader("n\n"))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdDestroy, "gone"})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if fp.destroyed != "" {
		t.Fatalf("destroyed = %q", fp.destroyed)
	}
	if !strings.Contains(out.String(), "cancelled") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestDestroyAll(t *testing.T) {
	fp := &fakeProvider{
		name: testSpritesName,
		list: []sandbox.Info{{ID: "a"}, {ID: "b"}},
	}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetIn(strings.NewReader("yes\n"))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdDestroy, testFlagAll})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(fp.destroyedIDs) != 2 || fp.destroyedIDs[0] != "a" || fp.destroyedIDs[1] != "b" {
		t.Fatalf("destroyed = %v", fp.destroyedIDs)
	}
	got := out.String()
	if !strings.Contains(got, "2 sandboxes were destroyed") || !strings.Contains(got, "  a") || !strings.Contains(got, "  b") {
		t.Fatalf("output = %q", got)
	}
}

func TestDestroyChooseFromList(t *testing.T) {
	fp := &fakeProvider{
		name: testSpritesName,
		list: []sandbox.Info{{ID: "a", Status: testStatusCold}, {ID: "b", Status: testStatusWarm}},
	}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetIn(strings.NewReader("2\n0\n"))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdDestroy})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if fp.destroyed != "b" {
		t.Fatalf("destroyed = %q", fp.destroyed)
	}
	if !strings.Contains(errOut.String(), "a") || !strings.Contains(errOut.String(), "b") {
		t.Fatalf("choices = %q", errOut.String())
	}
	if !strings.Contains(out.String(), "sandbox b was destroyed") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestDestroyAllEmpty(t *testing.T) {
	fp := &fakeProvider{name: testSpritesName}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdDestroy, testFlagAll})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if fp.destroyed != "" {
		t.Fatalf("destroyed = %q", fp.destroyed)
	}
	if !strings.Contains(out.String(), "no sandboxes to destroy") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRestartCancelledWithoutConfirm(t *testing.T) {
	fp := &fakeProvider{name: testSpritesName}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetIn(strings.NewReader("n\n"))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdRestart, testBoxName})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if fp.restarted != "" {
		t.Fatalf("restarted = %q", fp.restarted)
	}
	if !strings.Contains(out.String(), "cancelled") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRestartWithConfirm(t *testing.T) {
	testNamedActionConfirm(t, testCmdRestart, "restarted", func(fp *fakeProvider) sandbox.ID {
		return fp.restarted
	})
}

func TestRestartWithoutNameChoosesFromList(t *testing.T) {
	testActionFromList(t, testCmdRestart, "Restart which sandbox?", "sandbox b was restarted", func(fp *fakeProvider) sandbox.ID {
		return fp.restarted
	})
}

func TestRestartWithoutNameEmpty(t *testing.T) {
	testActionWhenEmpty(t, testCmdRestart, "no sandboxes to restart", func(fp *fakeProvider) sandbox.ID {
		return fp.restarted
	})
}

func TestUpgradeCancelledWithoutConfirm(t *testing.T) {
	fp := &fakeProvider{name: testSpritesName}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetIn(strings.NewReader("n\n"))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdUpgrade, testBoxName})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if fp.upgraded != "" {
		t.Fatalf("upgraded = %q", fp.upgraded)
	}
}

func TestUpgradeWithoutNameChoosesFromList(t *testing.T) {
	testActionFromList(t, testCmdUpgrade, "Upgrade which sandbox?", "sandbox b was upgraded", func(fp *fakeProvider) sandbox.ID {
		return fp.upgraded
	})
}

func TestUpgradeWithoutNameEmpty(t *testing.T) {
	testActionWhenEmpty(t, testCmdUpgrade, "no sandboxes to upgrade", func(fp *fakeProvider) sandbox.ID {
		return fp.upgraded
	})
}

func TestUpgradeWithConfirm(t *testing.T) {
	testNamedActionConfirm(t, testCmdUpgrade, "upgraded", func(fp *fakeProvider) sandbox.ID {
		return fp.upgraded
	})
}

func testNamedActionConfirm(t *testing.T, subcmd, needle string, acted func(*fakeProvider) sandbox.ID) {
	t.Helper()
	fp := &fakeProvider{name: testSpritesName}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetIn(strings.NewReader("y\n"))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, subcmd, testBoxName})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if acted(fp) != testBoxName {
		t.Fatalf("acted = %q", acted(fp))
	}
	if !strings.Contains(out.String(), needle) {
		t.Fatalf("output = %q", out.String())
	}
}

func testActionFromList(t *testing.T, subcmd, prompt, success string, acted func(*fakeProvider) sandbox.ID) {
	t.Helper()
	fp := &fakeProvider{
		name: testSpritesName,
		list: []sandbox.Info{{ID: "a", Status: testStatusCold}, {ID: "b", Status: testStatusWarm}},
	}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetIn(strings.NewReader("2\ny\n"))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, subcmd})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if acted(fp) != "b" {
		t.Fatalf("acted = %q", acted(fp))
	}
	if !strings.Contains(errOut.String(), prompt) {
		t.Fatalf("choices = %q", errOut.String())
	}
	if !strings.Contains(out.String(), success) {
		t.Fatalf("output = %q", out.String())
	}
}

func testActionWhenEmpty(t *testing.T, subcmd, emptyMsg string, acted func(*fakeProvider) sandbox.ID) {
	t.Helper()
	fp := &fakeProvider{name: testSpritesName}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, subcmd})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if acted(fp) != "" {
		t.Fatalf("acted = %q", acted(fp))
	}
	if !strings.Contains(out.String(), emptyMsg) {
		t.Fatalf("output = %q", out.String())
	}
}

func TestDestroyReturnsProviderError(t *testing.T) {
	fp := &fakeProvider{name: testSpritesName, destroyErr: errors.New("delete failed")}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetIn(strings.NewReader("y\n"))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdDestroy, "gone"})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("err = %v", err)
	}
}

func TestExecReturnsExitError(t *testing.T) {
	fp := &fakeProvider{
		name:    testSpritesName,
		execErr: &sandbox.ExitError{Code: 7},
	}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdExec, testArgBox, "false"})
	t.Setenv("SBOX_SPRITES_TOKEN", "tok")

	err := cmd.Execute()
	var exitErr *sandbox.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("want ExitError, got %v", err)
	}
	if exitErr.Code != 7 {
		t.Fatalf("code = %d", exitErr.Code)
	}
}

func TestSandboxSubcommandHelp(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testSpritesName}
	reg := spritesRegistry(t, fp)
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"create", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	help := out.String()
	for _, want := range []string{"--api", "--api-cmd", "--api-port", "--public", "0.0.0.0", "--sync", "--dir", "--running", "HTTP API", "30s"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q: %s", want, help)
		}
	}
}
