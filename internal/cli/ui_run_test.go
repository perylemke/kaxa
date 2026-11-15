package cli

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/config"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/web"
)

func TestRunUINeedsConfig(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	if err := runUI(cmd, registry.New(), 0, true); err == nil {
		t.Fatal("expected a missing configuration error")
	}
}

func TestRunUIReturnsServeError(t *testing.T) {
	prev := uiServe
	uiServe = func(context.Context, web.Options) error { return errors.New("bind failed") }
	t.Cleanup(func() { uiServe = prev })

	cmd := &cobra.Command{}
	cmd.Flags().String("provider", "", "")
	cmd.SetContext(withConfig(t.Context(), config.Config{Provider: spritesProviderName}))
	cmd.SetErr(io.Discard)
	if err := cmd.Flags().Set("provider", spritesProviderName); err != nil {
		t.Fatal(err)
	}
	err := runUI(cmd, registry.New(), 0, true)
	if err == nil || !strings.Contains(err.Error(), "bind failed") {
		t.Fatalf("err = %v", err)
	}
	if selectedProvider(cmd, config.Config{Provider: spritesProviderName}) != spritesProviderName {
		t.Fatal("provider flag was ignored")
	}
}

func TestSelectedProviderWithoutFlag(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	cmd.Flags().String("provider", "", "")
	if selectedProvider(cmd, config.Config{Provider: spritesProviderName}) != "" {
		t.Fatal("unset flag selected a provider")
	}
}
