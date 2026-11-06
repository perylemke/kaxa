package cli

import (
	"bytes"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

func TestCapabilityFieldHiddenSkipsViewAndRun(t *testing.T) {
	t.Parallel()

	val := false
	inner := huh.NewConfirm().Title("running").Value(&val)
	f := &capabilityField{inner: inner, show: func() bool { return false }}

	if f.View() != "" {
		t.Fatalf("View = %q, want empty", f.View())
	}
	if !f.Skip() {
		t.Fatal("Skip should be true when hidden")
	}
	if err := f.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := f.RunAccessible(new(bytes.Buffer), strings.NewReader("y\n")); err != nil {
		t.Fatalf("RunAccessible: %v", err)
	}
}

func TestCapabilityFieldVisibleDelegatesRun(t *testing.T) {
	t.Parallel()

	val := false
	inner := huh.NewConfirm().Title("x").Affirmative("Yes").Negative("No").Value(&val)
	f := &capabilityField{inner: inner, show: func() bool { return true }}

	out := new(bytes.Buffer)
	if err := f.RunAccessible(out, strings.NewReader("y\n")); err != nil {
		t.Fatalf("RunAccessible: %v", err)
	}
}

func TestCapabilityFieldKeyBindsAppendsQuit(t *testing.T) {
	t.Parallel()

	inner := huh.NewConfirm()
	f := &capabilityField{inner: inner, show: func() bool { return true }}
	binds := f.KeyBinds()
	if len(binds) < 2 {
		t.Fatalf("binds = %d, want at least 2", len(binds))
	}
	last := binds[len(binds)-1]
	keys := last.Keys()
	if len(keys) == 0 || keys[len(keys)-1] != "ctrl+c" {
		t.Fatalf("last binding keys = %#v, want ctrl+c", keys)
	}
}

func TestCapabilityFieldTeaDelegation(t *testing.T) {
	t.Parallel()

	val := false
	inner := huh.NewConfirm().Value(&val)
	f := &capabilityField{inner: inner, show: func() bool { return true }}
	if f.Init() != nil {
		t.Fatal("Init: unexpected cmd")
	}
	m, cmd := f.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	_ = m
	_ = cmd
	if f.Blur() == nil && f.Focus() == nil && f.Error() != nil {
		t.Fatal("unexpected Error state")
	}
	_ = f.Zoom()
}

func TestCapabilityFieldWithMethodsChain(t *testing.T) {
	t.Parallel()

	inner := huh.NewInput().Value(new(string))
	f := &capabilityField{inner: inner, show: func() bool { return true }}
	km := huh.NewDefaultKeyMap()
	var field huh.Field = f
	field = field.WithTheme(huh.ThemeFunc(huh.ThemeCharm))
	field = field.WithKeyMap(km)
	field = field.WithWidth(40)
	field = field.WithHeight(10)
	field = field.WithPosition(huh.FieldPosition{Group: 0, Field: 1, FirstField: 0, LastField: 2, GroupCount: 1, FirstGroup: 0, LastGroup: 0})
	if field.GetKey() == "" && field.GetValue() == nil {
		t.Fatal("expected GetKey or GetValue from inner")
	}
}
