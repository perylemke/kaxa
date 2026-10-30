package cli

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

func newSnapshotCmd(reg *registry.Registry) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "snapshot",
		Short:       "Save and restore the state of a sandbox",
		Annotations: featureCommand(sandbox.FeatureSnapshot),
		Long: `Save the filesystem of a sandbox and bring it back later.

Whether a restore overwrites the sandbox or creates a new one depends on the
provider: Sprites restores in place, Modal and Docker create a new sandbox with
--to and keep the source. A snapshot copies the whole filesystem, including any
secrets written inside the sandbox.`,
	}
	cmd.AddCommand(
		newSnapshotCreateCmd(reg),
		newSnapshotListCmd(reg),
		newSnapshotRestoreCmd(reg),
		newSnapshotDeleteCmd(reg),
	)
	return cmd
}

func newSnapshotCreateCmd(reg *registry.Registry) *cobra.Command {
	var comment string
	cmd := &cobra.Command{
		Use:     "create <name>",
		Short:   "Take a snapshot of a sandbox",
		Example: `  kaxa snapshot create my-sandbox --comment "before the migration"`,
		Args:    snapshotArgs("snapshot create", "kaxa snapshot create <name>", "a sandbox name", 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, s, err := snapshotterFromCmd(cmd, reg)
			if err != nil {
				return err
			}
			id := sandbox.ID(args[0])
			var snap sandbox.Snapshot
			label := fmt.Sprintf("snapshotting %s…", sandboxLabel(p.Name(), id))
			err = runTaskWithSpinner(cmd.Context(), cmd.ErrOrStderr(), label, func(ctx context.Context) error {
				var err error
				snap, err = s.Snapshot(ctx, id, comment)
				return err
			})
			if err != nil {
				return err
			}
			msg := fmt.Sprintf("snapshot %s of sandbox %s was created", snap.ID, sandboxLabel(p.Name(), id))
			return WriteSuccess(cmd.OutOrStdout(), "snapshot", msg)
		},
	}
	cmd.Flags().StringVar(&comment, "comment", "", "note to store with the snapshot")
	return cmd
}

func newSnapshotListCmd(reg *registry.Registry) *cobra.Command {
	return &cobra.Command{
		Use:     "list [name]",
		Aliases: []string{"ls"},
		Short:   "List snapshots of all providers, or of one sandbox",
		Long: `List snapshots with the provider and the sandbox each was taken from.

Without a name, it lists every snapshot of every configured provider. With a
name, it lists the snapshots of that sandbox in every provider that has it.
--provider limits the search to one provider.`,
		Example: `  kaxa snapshot list
  kaxa snapshot list my-sandbox
  kaxa snapshot list --provider modal`,
		Args: atMostOneName("snapshot list"),
		RunE: func(cmd *cobra.Command, args []string) error {
			var id sandbox.ID
			if len(args) == 1 {
				id = sandbox.ID(args[0])
			}
			providers, names, err := snapshotTargets(cmd, reg)
			if err != nil {
				return err
			}
			snaps, err := collectSnapshots(cmd.Context(), cmd.ErrOrStderr(), providers, names, id)
			if err != nil {
				return err
			}
			if len(snaps) == 0 {
				msg := "no snapshots"
				if id != "" {
					msg += " for " + string(id)
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), msg)
				return err
			}
			return writeSnapshots(cmd.OutOrStdout(), snaps)
		},
	}
}

// snapshotTargets is the provider named by --provider, which must support
// snapshots, or every configured provider.
func snapshotTargets(cmd *cobra.Command, reg *registry.Registry) (map[string]sandbox.Provider, []string, error) {
	if cmd.Flags().Changed("provider") {
		p, _, err := snapshotterFromCmd(cmd, reg)
		if err != nil {
			return nil, nil, err
		}
		return map[string]sandbox.Provider{p.Name(): p}, []string{p.Name()}, nil
	}
	return listTargets(cmd, reg)
}

// collectSnapshots lists the snapshots of every named provider. A provider that
// fails is reported on errOut and skipped while another succeeds.
func collectSnapshots(ctx context.Context, errOut io.Writer, providers map[string]sandbox.Provider, names []string, id sandbox.ID) ([]sandbox.Snapshot, error) {
	snaps, failures, err := lifecycle.CollectSnapshots(ctx, providers, names, id)
	if err != nil {
		return nil, err
	}
	for _, failure := range failures {
		if _, werr := fmt.Fprintf(errOut, "warning: could not list snapshots on %v\n", failure); werr != nil {
			return nil, werr
		}
	}
	return snaps, nil
}

func writeSnapshots(w io.Writer, snaps []sandbox.Snapshot) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "PROVIDER\tSANDBOX\tID\tCREATED\tCOMMENT"); err != nil {
		return err
	}
	for _, s := range snaps {
		created := ""
		if !s.CreatedAt.IsZero() {
			created = s.CreatedAt.UTC().Format(time.RFC3339)
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", plainText(s.Provider), plainText(string(s.Sandbox)), plainText(string(s.ID)), created, plainText(s.Comment)); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func newSnapshotRestoreCmd(reg *registry.Registry) *cobra.Command {
	var to string
	var force bool
	cmd := &cobra.Command{
		Use:   "restore <name> <snapshot-id>",
		Short: "Restore a snapshot",
		Long: `Restore a snapshot of a sandbox.

Sprites restores the sandbox itself and discards what changed after the
snapshot, so it asks first. Modal and Docker create a new sandbox named by --to
and keep the source.`,
		Example: `  kaxa snapshot restore my-sprite v3 --provider sprites
  kaxa snapshot restore my-sandbox im-abc123 --to my-copy --provider modal
  kaxa snapshot restore my-sandbox 0abc123 --to my-copy --provider docker`,
		Args: snapshotArgs("snapshot restore", "kaxa snapshot restore <name> <snapshot-id> [--to <new-name>]", "a sandbox name and a snapshot id", 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, s, err := snapshotterFromCmd(cmd, reg)
			if err != nil {
				return err
			}
			req := sandbox.RestoreRequest{
				Sandbox:  sandbox.ID(args[0]),
				Snapshot: sandbox.SnapshotID(args[1]),
				Target:   sandbox.ID(to),
			}
			if err := lifecycle.ValidateSnapshotRequest(s.RestoreMode(), req); err != nil {
				return err
			}
			return runRestore(cmd, p, s, req, force)
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "name of the new sandbox (providers that restore into a new sandbox)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "restore in place without confirmation")
	markProviderFlags(cmd, "to")
	return cmd
}

func runRestore(cmd *cobra.Command, p sandbox.Provider, s sandbox.Snapshotter, req sandbox.RestoreRequest, force bool) error {
	src := sandboxLabel(p.Name(), req.Sandbox)
	run := func(ctx context.Context) error {
		_, err := s.Restore(ctx, req)
		return err
	}
	if s.RestoreMode() == sandbox.RestoreNewSandbox {
		label := fmt.Sprintf("restoring %s as %s…", req.Snapshot, req.Target)
		if err := runTaskWithSpinner(cmd.Context(), cmd.ErrOrStderr(), label, run); err != nil {
			return err
		}
		msg := fmt.Sprintf("snapshot %s of sandbox %s was restored as %s; the source was kept",
			req.Snapshot, src, sandboxLabel(p.Name(), req.Target))
		return WriteSuccess(cmd.OutOrStdout(), "restored", msg)
	}
	label := fmt.Sprintf("restoring %s…", src)
	msg := fmt.Sprintf("sandbox %s was restored to snapshot %s", src, req.Snapshot)
	if force {
		if err := runTaskWithSpinner(cmd.Context(), cmd.ErrOrStderr(), label, run); err != nil {
			return err
		}
		return WriteSuccess(cmd.OutOrStdout(), "restored", msg)
	}
	prompt := fmt.Sprintf("Restore sandbox %s to snapshot %s? Changes made after it are lost.", src, req.Snapshot)
	return runConfirmedSandboxAction(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr(), cmd.OutOrStdout(),
		prompt, label, "restored", msg, run)
}

func newSnapshotDeleteCmd(reg *registry.Registry) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:     "delete <name> <snapshot-id>",
		Aliases: []string{"rm"},
		Short:   "Delete a snapshot",
		Example: `  kaxa snapshot delete my-sandbox v3`,
		Args:    snapshotArgs("snapshot delete", "kaxa snapshot delete <name> <snapshot-id>", "a sandbox name and a snapshot id", 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, s, err := snapshotterFromCmd(cmd, reg)
			if err != nil {
				return err
			}
			id, snap := sandbox.ID(args[0]), sandbox.SnapshotID(args[1])
			if err := lifecycle.ValidateSnapshotID(snap); err != nil {
				return err
			}
			ref := sandboxLabel(p.Name(), id)
			label := fmt.Sprintf("deleting snapshot %s of %s…", snap, ref)
			run := func(ctx context.Context) error { return s.DeleteSnapshot(ctx, id, snap) }
			msg := fmt.Sprintf("snapshot %s of sandbox %s was deleted", snap, ref)
			if force {
				if err := runTaskWithSpinner(cmd.Context(), cmd.ErrOrStderr(), label, run); err != nil {
					return err
				}
				return WriteSuccess(cmd.OutOrStdout(), "deleted", msg)
			}
			return runConfirmedSandboxAction(cmd.Context(), cmd.InOrStdin(), cmd.ErrOrStderr(), cmd.OutOrStdout(),
				fmt.Sprintf("Delete snapshot %s of sandbox %s?", snap, ref), label, "deleted", msg, run)
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "delete without confirmation")
	return cmd
}

// snapshotterFromCmd resolves the provider and refuses one without snapshots
// before any request leaves the machine.
func snapshotterFromCmd(cmd *cobra.Command, reg *registry.Registry) (sandbox.Provider, sandbox.Snapshotter, error) {
	p, err := providerFromCmd(cmd, reg)
	if err != nil {
		return nil, nil, err
	}
	s, err := lifecycle.SnapshotterOf(p)
	if err != nil {
		return nil, nil, err
	}
	return p, s, nil
}

// snapshotArgs demands the exact positional arguments of a snapshot command.
func snapshotArgs(command, usage, what string, n int) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		switch {
		case len(args) < n:
			return usererr.New(command+" requires "+what, usage)
		case len(args) > n:
			return usererr.New(command+" accepts only "+what, usage)
		}
		return nil
	}
}
