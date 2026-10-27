package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

var errDestroyCancelled = errors.New("destroy cancelled")

func newDestroyCmd(reg *registry.Registry) *cobra.Command {
	var destroyAll bool
	var force bool

	cmd := &cobra.Command{
		Use:     "destroy [name]",
		Aliases: []string{"rm"},
		Short:   "Destroy a sandbox",
		Args: func(_ *cobra.Command, args []string) error {
			return destroyArgError(destroyAll, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := providerFromCmd(cmd, reg)
			if err != nil {
				return err
			}

			ids, picked, err := destroyTargets(cmd.Context(), p, destroyAll, args, cmd.InOrStdin(), cmd.ErrOrStderr())
			if errors.Is(err, errDestroyCancelled) {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "destroy cancelled")
				return err
			}
			if err != nil {
				return err
			}
			if len(ids) == 0 {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "no sandboxes to destroy")
				return err
			}

			if !picked && !force {
				ok, err := confirmAction(cmd.InOrStdin(), cmd.ErrOrStderr(), destroyPrompt(destroyAll, ids))
				if err != nil {
					return err
				}
				if !ok {
					_, err = fmt.Fprintln(cmd.OutOrStdout(), "destroy cancelled")
					return err
				}
			}

			err = runTaskWithSpinner(cmd.Context(), cmd.ErrOrStderr(), destroySpinnerLabel(ids), func(ctx context.Context) error {
				for _, id := range ids {
					if err := p.Destroy(ctx, id); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
			return WriteSuccess(cmd.OutOrStdout(), "destroyed", destroySuccessMessage(ids))
		},
	}
	cmd.Flags().BoolVar(&destroyAll, "all", false, "destroy every sandbox")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "destroy without confirmation")
	return cmd
}

func destroyArgError(destroyAll bool, args []string) error {
	if destroyAll {
		if len(args) == 0 {
			return nil
		}
		return usererr.New(
			"destroy --all does not take a sandbox name",
			"kaxa destroy --all",
		)
	}
	return rejectExtraName("destroy", args)
}

func destroyTargets(ctx context.Context, p sandbox.Provider, destroyAll bool, args []string, in io.Reader, errOut io.Writer) ([]sandbox.ID, bool, error) {
	if !destroyAll && len(args) == 1 {
		return []sandbox.ID{sandbox.ID(args[0])}, false, nil
	}
	infos, err := p.List(ctx)
	if err != nil {
		return nil, false, err
	}
	if len(infos) == 0 {
		return nil, false, nil
	}
	if destroyAll {
		ids := make([]sandbox.ID, 0, len(infos))
		for _, info := range infos {
			ids = append(ids, info.ID)
		}
		return ids, false, nil
	}
	ids, err := collectDestroyForm(in, errOut, infos)
	return ids, true, err
}

func chooseSandbox(in io.Reader, out io.Writer, prompt string, infos []sandbox.Info) (sandbox.ID, error) {
	for i, info := range infos {
		if _, err := fmt.Fprintf(out, "  %d  %s\t%s\n", i+1, info.ID, info.Status); err != nil {
			return "", err
		}
	}
	if _, err := fmt.Fprint(out, prompt); err != nil {
		return "", err
	}
	var answer string
	if _, err := fmt.Fscanln(in, &answer); err != nil {
		return "", err
	}
	answer = strings.TrimSpace(answer)
	if n, err := strconv.Atoi(answer); err == nil {
		if n < 1 || n > len(infos) {
			return "", unknownSandboxChoice(answer)
		}
		return infos[n-1].ID, nil
	}
	for _, info := range infos {
		if string(info.ID) == answer {
			return info.ID, nil
		}
	}
	return "", unknownSandboxChoice(answer)
}

func unknownSandboxChoice(answer string) error {
	return usererr.New(fmt.Sprintf("unknown sandbox %q", answer), "enter a number from the list")
}

func destroyPrompt(destroyAll bool, ids []sandbox.ID) string {
	if destroyAll {
		return fmt.Sprintf("Destroy all %d sandboxes?", len(ids))
	}
	return fmt.Sprintf("Destroy sandbox %s?", ids[0])
}

func destroySpinnerLabel(ids []sandbox.ID) string {
	if len(ids) > 1 {
		return fmt.Sprintf("destroying %d sandboxes…", len(ids))
	}
	return fmt.Sprintf("destroying %s…", ids[0])
}

func destroySuccessMessage(ids []sandbox.ID) string {
	if len(ids) == 1 {
		return fmt.Sprintf("sandbox %s was destroyed", ids[0])
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d sandboxes were destroyed", len(ids))
	for _, id := range ids {
		b.WriteString("\n  ")
		b.WriteString(string(id))
	}
	return b.String()
}

func confirmAction(in io.Reader, out io.Writer, prompt string) (bool, error) {
	if _, err := fmt.Fprintf(out, "%s [y/N]: ", prompt); err != nil {
		return false, err
	}
	var answer string
	if _, err := fmt.Fscanln(in, &answer); err != nil {
		answer = "n"
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
