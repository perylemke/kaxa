package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
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
		Short:   "Destroy sandboxes",
		Long: `Destroy sandboxes.

With a name, destroy that sandbox on the selected provider.
With no name, choose from the sandboxes of every configured provider.
With --all, destroy every sandbox of every configured provider, after two
confirmations. Add --provider to limit the scope to one provider.`,
		Args: func(_ *cobra.Command, args []string) error {
			return destroyArgError(destroyAll, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			plan, err := resolveDestroyPlan(cmd, reg, destroyAll, args)
			if errors.Is(err, errDestroyCancelled) {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "destroy cancelled")
				return err
			}
			if err != nil {
				return err
			}
			if len(plan.targets) == 0 {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "no sandboxes to destroy")
				return err
			}

			if !plan.picked && !force {
				ok, err := confirmDestroy(cmd, plan, destroyAll)
				if err != nil {
					return err
				}
				if !ok {
					_, err = fmt.Fprintln(cmd.OutOrStdout(), "destroy cancelled")
					return err
				}
			}

			err = runTaskWithSpinner(cmd.Context(), cmd.ErrOrStderr(), destroySpinnerLabel(plan.targets), plan.run)
			if err != nil {
				return err
			}
			return WriteSuccess(cmd.OutOrStdout(), "destroyed", destroySuccessMessage(plan.targets))
		},
	}
	cmd.Flags().BoolVar(&destroyAll, "all", false, "destroy every sandbox of every provider (or of --provider)")
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

// destroyPlan is what a destroy run will remove and the providers to ask.
type destroyPlan struct {
	targets   []sandbox.Info
	providers map[string]sandbox.Provider
	// picked means the user chose the targets in the form, which is the confirmation.
	picked bool
	// allProviders means --all with no --provider, the widest scope.
	allProviders bool
}

func (d destroyPlan) run(ctx context.Context) error {
	for _, t := range d.targets {
		if err := d.providers[t.Provider].Destroy(ctx, t.ID); err != nil {
			if len(d.targets) == 1 {
				return err
			}
			return fmt.Errorf("destroy %s: %w", sandboxLabel(t.Provider, t.ID), err)
		}
	}
	return nil
}

func resolveDestroyPlan(cmd *cobra.Command, reg *registry.Registry, destroyAll bool, args []string) (destroyPlan, error) {
	if !destroyAll && len(args) == 1 {
		p, err := providerFromCmd(cmd, reg)
		if err != nil {
			return destroyPlan{}, err
		}
		return destroyPlan{
			targets:   []sandbox.Info{{Provider: p.Name(), ID: sandbox.ID(args[0])}},
			providers: map[string]sandbox.Provider{p.Name(): p},
		}, nil
	}

	providers, names, err := listTargets(cmd, reg)
	if err != nil {
		return destroyPlan{}, err
	}
	infos, err := collectSandboxes(cmd.Context(), cmd.ErrOrStderr(), providers, names)
	if err != nil {
		return destroyPlan{}, err
	}
	plan := destroyPlan{providers: providers}
	if len(infos) == 0 {
		return plan, nil
	}
	if destroyAll {
		plan.targets = infos
		plan.allProviders = !cmd.Flags().Changed("provider")
		return plan, nil
	}
	plan.targets, err = collectDestroyForm(cmd.InOrStdin(), cmd.ErrOrStderr(), infos)
	plan.picked = true
	return plan, err
}

// confirmDestroy asks before destroying. --all across every provider asks twice:
// the second is a last warning.
func confirmDestroy(cmd *cobra.Command, plan destroyPlan, destroyAll bool) (bool, error) {
	in, errOut := cmd.InOrStdin(), cmd.ErrOrStderr()
	ok, err := confirmAction(in, errOut, destroyPrompt(plan.targets, destroyAll))
	if err != nil || !ok || !plan.allProviders {
		return ok, err
	}
	return confirmAction(in, errOut, destroyFinalPrompt(plan.targets))
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

func destroyPrompt(targets []sandbox.Info, destroyAll bool) string {
	if destroyAll {
		return fmt.Sprintf("Destroy %s %s?", allSandboxes(len(targets)), providersPhrase(targets))
	}
	return fmt.Sprintf("Destroy sandbox %s?", sandboxLabel(targets[0].Provider, targets[0].ID))
}

func destroyFinalPrompt(targets []sandbox.Info) string {
	return fmt.Sprintf("This cannot be undone. Really destroy %s %s?", allSandboxes(len(targets)), providersPhrase(targets))
}

func destroySpinnerLabel(targets []sandbox.Info) string {
	if len(targets) > 1 {
		return fmt.Sprintf("destroying %d sandboxes %s…", len(targets), providersPhrase(targets))
	}
	return fmt.Sprintf("destroying %s…", sandboxLabel(targets[0].Provider, targets[0].ID))
}

func destroySuccessMessage(targets []sandbox.Info) string {
	if len(targets) == 1 {
		return fmt.Sprintf("sandbox %s was destroyed", sandboxLabel(targets[0].Provider, targets[0].ID))
	}
	multi := len(providerNames(targets)) > 1
	var b strings.Builder
	fmt.Fprintf(&b, "%d sandboxes %s were destroyed", len(targets), providersPhrase(targets))
	for _, t := range targets {
		b.WriteString("\n  ")
		if multi {
			b.WriteString(sandboxLabel(t.Provider, t.ID))
			continue
		}
		b.WriteString(string(t.ID))
	}
	return b.String()
}

// allSandboxes reads "all 3 sandboxes", or "1 sandbox" when there is only one.
func allSandboxes(n int) string {
	if n == 1 {
		return "1 sandbox"
	}
	return fmt.Sprintf("all %d sandboxes", n)
}

// providerNames returns the distinct providers of targets, in first-seen order.
func providerNames(targets []sandbox.Info) []string {
	var names []string
	for _, t := range targets {
		if !slices.Contains(names, t.Provider) {
			names = append(names, t.Provider)
		}
	}
	return names
}

// providersPhrase reads "on modal" for one provider, "across modal, sprites" for more.
func providersPhrase(targets []sandbox.Info) string {
	names := providerNames(targets)
	if len(names) == 1 {
		return "on " + names[0]
	}
	return "across " + strings.Join(names, ", ")
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

// sandboxLabel names a sandbox with its provider, as in "demo on modal".
func sandboxLabel(provider string, id sandbox.ID) string {
	return fmt.Sprintf("%s on %s", id, provider)
}
