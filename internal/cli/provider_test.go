package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/registry"
)

func TestProviderFromCmdMissingConfigInContext(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	_, err := providerFromCmd(cmd, registry.New())
	if err == nil || !strings.Contains(err.Error(), "not loaded") {
		t.Fatalf("err = %v", err)
	}
}
