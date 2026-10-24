package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/perylemke/sbox/internal/registry"
	"github.com/perylemke/sbox/internal/sandbox"
)

var sboxCreateLabels = []string{"sbox"}

func newCreateCmd(reg *registry.Registry) *cobra.Command {
	var keepRunning bool

	cmd := &cobra.Command{
		Use:     "create [name]",
		Aliases: []string{"new"},
		Short:   "Create a sandbox",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			running := keepRunning
			if len(args) == 1 {
				name = args[0]
			} else {
				cfg, err := configFromContext(cmd.Context())
				if err != nil {
					return err
				}
				form, err := collectCreateForm(
					cmd.InOrStdin(),
					cmd.ErrOrStderr(),
					reg.Names(),
					cfg.Provider,
					keepRunning,
				)
				if errors.Is(err, errCreateCancelled) {
					_, err = fmt.Fprintln(cmd.OutOrStdout(), "create cancelled")
					return err
				}
				if err != nil {
					return err
				}
				name = form.Name
				running = form.Running
				cfg.Provider = form.Provider
				cmd.SetContext(withConfig(cmd.Context(), cfg))
			}

			p, err := providerFromCmd(cmd, reg)
			if err != nil {
				return err
			}
			spec := sandbox.Spec{Name: name, Labels: sboxCreateLabels, Running: running}
			id, err := runCreateWithSpinner(cmd.Context(), cmd.ErrOrStderr(), name, func(ctx context.Context) (sandbox.ID, error) {
				return p.Create(ctx, spec)
			})
			if err != nil {
				return err
			}
			msg := fmt.Sprintf("sandbox %s is ready", id)
			if running {
				detail, inspectErr := p.Inspect(cmd.Context(), id)
				if inspectErr != nil {
					return inspectErr
				}
				msg = fmt.Sprintf("sandbox %s is ready and will stay running for 1 hour\n%s", id, detail.URL)
				_ = openBrowserFn(detail.URL)
			}
			return WriteSuccess(cmd.OutOrStdout(), "created", msg)
		},
	}
	cmd.Flags().BoolVar(&keepRunning, "running", false, "keep the sandbox running for 1 hour and open a tmux terminal in the browser")
	return cmd
}
