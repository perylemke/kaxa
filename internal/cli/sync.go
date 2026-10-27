package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/reposync"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

func newSyncCmd(reg *registry.Registry) *cobra.Command {
	var dir string

	cmd := &cobra.Command{
		Use:   "sync <name>",
		Short: "Copy another Git repository into a sandbox",
		Long: `Copy a Git repository into a sandbox that was created with kaxa create --sync, then install its dependencies.

The repository lands in /home/sprite/workspace/<repo> on Sprites or /home/modal/workspace/<repo> on Modal, including .git and excluding ignored files. --dir selects the repository; without it, sync uses the current directory. A subdirectory still copies the whole Git repository.
A sandbox created without --sync has no workspace and is rejected, because a Modal sandbox picks its image when it is created.
If <repo> already exists in the sandbox, sync stops before copying and does not replace it. A failed copy or install keeps the sandbox.`,
		Example: `  kaxa sync my-sprite
  kaxa sync --dir ~/projects/api-python my-sprite
  kaxa --provider modal sync my-box`,
		Args: requireSyncName,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSync(cmd, reg, args[0], dir, cmd.Flags().Changed("dir"))
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Git repository to copy (default: the current directory)")
	return cmd
}

func runSync(cmd *cobra.Command, reg *registry.Registry, name, dir string, dirSet bool) error {
	root, repo, err := resolveSyncRepo(dir, dirSet)
	if err != nil {
		return err
	}
	p, err := providerFromCmd(cmd, reg)
	if err != nil {
		return err
	}
	errOut := cmd.ErrOrStderr()
	res, err := lifecycle.Sync(cmd.Context(), p, lifecycle.SyncRequest{Name: name, Root: root, Repo: repo}, func(ctx context.Context, label string, task func(context.Context) error) error {
		return runTaskWithSpinner(ctx, errOut, label, task)
	})
	if err != nil {
		return err
	}
	return WriteSuccess(cmd.OutOrStdout(), "synced", syncedMessage(p, res, repo))
}

// resolveSyncRepo finds the repository to copy. It words the hint for sync,
// not for create, when the current directory is not a repository.
func resolveSyncRepo(dir string, dirSet bool) (string, string, error) {
	root, repo, err := lifecycle.ResolveRepo(dir, dirSet)
	if err != nil && !dirSet && errors.Is(err, reposync.ErrNotRepository) {
		return "", "", usererr.Wrap(
			err,
			"current directory is not inside a git repository",
			"run kaxa sync from a git repository, or pass --dir",
		)
	}
	return root, repo, err
}

func syncedMessage(p sandbox.Provider, res lifecycle.Result, repo string) string {
	ref := sandboxLabel(p.Name(), res.ID)
	if res.WorkDir == "" {
		return fmt.Sprintf("%s copied to sandbox %s", repo, ref)
	}
	return fmt.Sprintf("%s copied to sandbox %s at %s", repo, ref, res.WorkDir)
}

func requireSyncName(_ *cobra.Command, args []string) error {
	switch len(args) {
	case 1:
		return nil
	case 0:
		return usererr.New("sync requires a sandbox name", "kaxa sync <name>")
	default:
		return usererr.New("sync accepts one sandbox name", "kaxa sync <name>")
	}
}
