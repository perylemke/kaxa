package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/perylemke/sbox/internal/sandbox"
)

func TestURLSetsSpritesAuth(t *testing.T) {
	const spriteURL = "https://example.sprites.app"
	fp := &urlProvider{
		fakeProvider: &fakeProvider{
			name:   testSpritesName,
			detail: sandbox.Detail{ID: testBoxName, URL: spriteURL, Auth: sandbox.URLAuthSprite},
		},
	}
	cmd := newCreateCmd(t, fp)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdURL, testBoxName, sandbox.URLAuthSprite})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if fp.auth != sandbox.URLAuthSprite {
		t.Fatalf("auth = %q", fp.auth)
	}
	got := out.String()
	if !strings.Contains(got, spriteURL) || !strings.Contains(got, sandbox.URLAuthSprite) {
		t.Fatalf("output = %q", got)
	}
}

func TestURLRejectsProviderWithoutAuth(t *testing.T) {
	fp := &fakeProvider{name: testSpritesName}
	cmd := newCreateCmd(t, fp)
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdURL, testBoxName, sandbox.URLAuthPublic})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "does not support url auth") {
		t.Fatalf("err = %v", err)
	}
}

func TestURLRejectsUnknownAuth(t *testing.T) {
	fp := &urlProvider{fakeProvider: &fakeProvider{name: testSpritesName}}
	cmd := newCreateCmd(t, fp)
	cmd.SetArgs([]string{testProviderFlag, testSpritesName, testCmdURL, testBoxName, "open"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid url auth") {
		t.Fatalf("err = %v", err)
	}
	if fp.auth != "" {
		t.Fatalf("auth = %q", fp.auth)
	}
}

type urlProvider struct {
	*fakeProvider
	auth string
}

func (u *urlProvider) SetURLAuth(_ context.Context, _ sandbox.ID, auth string) error {
	u.auth = auth
	return nil
}
