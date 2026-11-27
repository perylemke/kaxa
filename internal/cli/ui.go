package cli

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/config"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/usererr"
	"github.com/perylemke/kaxa/internal/web"
)

func newUICmd(reg *registry.Registry) *cobra.Command {
	var port int
	var noOpen bool

	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Open a local page to manage sandboxes",
		Long: `Open a local page to list, create, restart, upgrade, change URL auth, and destroy sandboxes.

The page is served on 127.0.0.1. The port is chosen by the system unless --port is set, and the address is printed on stderr. Ctrl+C stops the server.

--provider limits the page to one provider. Without it, every provider is listed, and one that is not available is shown disabled.

Provider tokens stay in this process. They are not sent to the browser.`,
		Example: `  kaxa ui
  kaxa ui --no-open
  kaxa ui --port 4000
  kaxa ui --provider sprites`,
		Args: noArgs("ui"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUI(cmd, reg, port, noOpen)
		},
	}
	cmd.Flags().IntVar(&port, "port", 0, "port on 127.0.0.1 (0 picks a free port)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "do not open the browser")
	return cmd
}

func runUI(cmd *cobra.Command, reg *registry.Registry, port int, noOpen bool) error {
	if err := validateUIPort(port); err != nil {
		return err
	}
	cfg, err := configFromContext(cmd.Context())
	if err != nil {
		return err
	}
	ctx, stop := signalContext(cmd.Context(), cmd.ErrOrStderr())
	defer stop()
	err = uiServe(ctx, web.Options{
		Registry: reg,
		Config:   cfg,
		Provider: selectedProvider(cmd, cfg),
		Port:     port,
		Stderr:   cmd.ErrOrStderr(),
		Open:     browserForUI(noOpen),
	})
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func validateUIPort(port int) error {
	if port < 0 || port > 65535 {
		return usererr.New(
			"invalid port",
			"use a port from 0 to 65535; 0 picks a free port",
		)
	}
	return nil
}

func selectedProvider(cmd *cobra.Command, cfg config.Config) string {
	if cmd.Flags().Changed("provider") {
		return cfg.Provider
	}
	return ""
}

func browserForUI(noOpen bool) func(string) error {
	if noOpen {
		return nil
	}
	return openLoopbackURL
}

// uiServe starts the local UI. Tests may replace it.
var uiServe = web.ListenAndServe
