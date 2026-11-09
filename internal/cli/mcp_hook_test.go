package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/config"
	"github.com/perylemke/kaxa/internal/registry"
)

func TestRunMCPSwallowsCanceled(t *testing.T) {
	restore := SetMCPServeForTest(func(context.Context, *registry.Registry, config.Config, io.Writer) error {
		return context.Canceled
	})
	t.Cleanup(restore)

	cmd := &cobra.Command{}
	cmd.SetContext(withConfig(context.Background(), config.Config{Provider: spritesProviderName}))
	cmd.SetErr(new(bytes.Buffer))

	if err := runMCP(cmd, registry.New()); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestRunMCPPropagatesOtherErrors(t *testing.T) {
	want := errors.New("serve failed")
	restore := SetMCPServeForTest(func(context.Context, *registry.Registry, config.Config, io.Writer) error {
		return want
	})
	t.Cleanup(restore)

	cmd := &cobra.Command{}
	cmd.SetContext(withConfig(context.Background(), config.Config{Provider: spritesProviderName}))
	cmd.SetErr(new(bytes.Buffer))

	err := runMCP(cmd, registry.New())
	if !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}
