package cli

import (
	"context"
	"io"
	"os/exec"

	tea "charm.land/bubbletea/v2"

	"github.com/perylemke/kaxa/internal/config"
	"github.com/perylemke/kaxa/internal/registry"
)

// SetOpenBrowserForTest replaces the browser opener until the returned function runs.
func SetOpenBrowserForTest(fn func(string) error) func() {
	prev := openBrowserFn
	if fn == nil {
		fn = func(string) error { return nil }
	}
	openBrowserFn = fn
	return func() { openBrowserFn = prev }
}

// SetCreateWizardExtraTeaOptsForTest replaces createWizardExtraTeaOpts until cleanup runs.
func SetCreateWizardExtraTeaOptsForTest(opts []tea.ProgramOption) func() {
	prev := createWizardExtraTeaOpts
	createWizardExtraTeaOpts = append([]tea.ProgramOption(nil), opts...)
	return func() { createWizardExtraTeaOpts = prev }
}

// SetMCPServeForTest replaces mcpServe until cleanup runs.
func SetMCPServeForTest(fn func(context.Context, *registry.Registry, config.Config, io.Writer) error) func() {
	prev := mcpServe
	mcpServe = fn
	return func() { mcpServe = prev }
}

// SetBrowserExecCommandForTest replaces browserExecCommand until cleanup runs.
func SetBrowserExecCommandForTest(fn func(string, ...string) *exec.Cmd) func() {
	prev := browserExecCommand
	browserExecCommand = fn
	return func() { browserExecCommand = prev }
}
