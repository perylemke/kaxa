package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/cli"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
)

// destroyLog records "provider/id" for every Destroy across both fakes.
type destroyLog struct{ calls []string }

type destroySprites struct {
	listSprites
	log *destroyLog
}

func (d *destroySprites) Destroy(_ context.Context, id sandbox.ID) error {
	d.log.calls = append(d.log.calls, testSpritesName+"/"+string(id))
	return nil
}

type destroyModal struct {
	listModal
	log *destroyLog
}

func (d *destroyModal) Destroy(_ context.Context, id sandbox.ID) error {
	d.log.calls = append(d.log.calls, testModalName+"/"+string(id))
	return nil
}

func runDestroy(t *testing.T, stdin string, args ...string) (log *destroyLog, stdout, stderr string, err error) {
	t.Helper()
	log = &destroyLog{}
	sp, md := newListFakes()
	reg := registry.New()
	for _, p := range []sandbox.Provider{&destroySprites{listSprites: *sp, log: log}, &destroyModal{listModal: *md, log: log}} {
		if regErr := reg.Register(p); regErr != nil {
			t.Fatalf("register: %v", regErr)
		}
	}
	t.Setenv("KAXA_SPRITES_TOKEN", listToken)
	t.Setenv("KAXA_MODAL_TOKEN_ID", "ak-1")
	t.Setenv("KAXA_MODAL_TOKEN_SECRET", "as-2")
	t.Setenv("KAXA_DEFAULT_PROVIDER", testSpritesName)

	cmd := cli.NewRootCmd(reg)
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(append([]string{testCmdDestroy}, args...))
	err = cmd.Execute()
	return log, out.String(), errOut.String(), err
}

func TestDestroyAllAsksTwiceAcrossProviders(t *testing.T) {
	log, out, errOut, err := runDestroy(t, "yes\ny\n", testFlagAll)
	if err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if got := strings.Join(log.calls, ","); got != "modal/m1,sprites/s1" && got != "sprites/s1,modal/m1" {
		t.Fatalf("destroyed = %v", log.calls)
	}
	if n := strings.Count(errOut, "[y/N]"); n != 2 {
		t.Fatalf("prompts = %d in %q, want 2", n, errOut)
	}
	for _, want := range []string{"Destroy all 2 sandboxes across", "This cannot be undone"} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("stderr missing %q: %q", want, errOut)
		}
	}
	if !strings.Contains(out, "2 sandboxes across") || !strings.Contains(out, "m1 on modal") || !strings.Contains(out, "s1 on sprites") {
		t.Fatalf("output = %q", out)
	}
}

func TestDestroyAllCancelledAtEitherPrompt(t *testing.T) {
	tests := []struct {
		name  string
		stdin string
	}{
		{"first", "n\n"},
		{"second", "y\nn\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, out, _, err := runDestroy(t, tt.stdin, testFlagAll)
			if err != nil {
				t.Fatalf("destroy: %v", err)
			}
			if len(log.calls) != 0 {
				t.Fatalf("destroyed = %v", log.calls)
			}
			if !strings.Contains(out, "destroy cancelled") {
				t.Fatalf("output = %q", out)
			}
		})
	}
}

func TestDestroyAllForceSkipsBothPrompts(t *testing.T) {
	log, _, errOut, err := runDestroy(t, "", testFlagAll, "-f")
	if err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if len(log.calls) != 2 {
		t.Fatalf("destroyed = %v", log.calls)
	}
	if strings.Contains(errOut, "[y/N]") {
		t.Fatalf("prompt = %q", errOut)
	}
}

func TestDestroyAllWithProviderFlagStaysInThatProvider(t *testing.T) {
	log, out, errOut, err := runDestroy(t, "y\n", testFlagAll, testProviderFlag, testModalName)
	if err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if got := strings.Join(log.calls, ","); got != "modal/m1" {
		t.Fatalf("destroyed = %v", log.calls)
	}
	if n := strings.Count(errOut, "[y/N]"); n != 1 {
		t.Fatalf("prompts = %d, want 1", n)
	}
	if !strings.Contains(out, "sandbox m1 on modal was destroyed") {
		t.Fatalf("output = %q", out)
	}
}

func TestDestroyWithoutNameListsEveryProvider(t *testing.T) {
	// Accessible form: "2" toggles the second row, "0" confirms.
	log, _, errOut, err := runDestroy(t, "2\n0\n")
	if err != nil {
		t.Fatalf("destroy: %v", err)
	}
	for _, want := range []string{"m1  running  modal", "s1  warm  sprites"} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("form missing %q: %q", want, errOut)
		}
	}
	if len(log.calls) != 1 {
		t.Fatalf("destroyed = %v", log.calls)
	}
}

func TestDestroyPromptSingularForOneSandbox(t *testing.T) {
	sp, md := newListFakes()
	md.infos = nil
	log := &destroyLog{}
	reg := registry.New()
	for _, p := range []sandbox.Provider{&destroySprites{listSprites: *sp, log: log}, &destroyModal{listModal: *md, log: log}} {
		if err := reg.Register(p); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	t.Setenv("KAXA_SPRITES_TOKEN", listToken)
	t.Setenv("KAXA_MODAL_TOKEN_ID", "ak-1")
	t.Setenv("KAXA_MODAL_TOKEN_SECRET", "as-2")
	t.Setenv("KAXA_DEFAULT_PROVIDER", "")

	cmd := cli.NewRootCmd(reg)
	errOut := new(bytes.Buffer)
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(errOut)
	cmd.SetIn(strings.NewReader("n\n"))
	cmd.SetArgs([]string{testCmdDestroy, testFlagAll})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if !strings.Contains(errOut.String(), "Destroy 1 sandbox on sprites?") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}
