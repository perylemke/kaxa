package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/config"
)

func TestConfigFromContextMissing(t *testing.T) {
	t.Parallel()

	_, err := configFromContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not loaded") {
		t.Fatalf("err = %v", err)
	}
}

func TestConfigFromContextRoundTrip(t *testing.T) {
	t.Parallel()

	cfg := config.Config{Provider: spritesProviderName}
	ctx := withConfig(context.Background(), cfg)
	got, err := configFromContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != spritesProviderName {
		t.Fatalf("provider = %q", got.Provider)
	}
}
