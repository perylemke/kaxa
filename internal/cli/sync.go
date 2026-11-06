package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/reposync"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

func newSyncCmd(reg *registry.Registry) *cobra.Command {
	var dir string
	var noInstall bool

	cmd := &cobra.Command{
		Use:   "sync <name>",
		Short: "Copy another Git repository into a sandbox",
		Long: `Copy a Git repository into a sandbox, then install its dependencies.

The repository lands in the workspace of the sandbox, including .git and excluding ignored files: /home/sprite/workspace/<repo> on Sprites, /home/modal/workspace/<repo> on Modal, /home/user/workspace/<repo> on E2B, /home/agent/workspace/<repo> on Docker, and /home/daytona/workspace/<repo> on Daytona. --dir selects the repository; without it, sync uses the current directory. A subdirectory still copies the whole Git repository.
Every sandbox has a workspace, so one created without --sync accepts sync too, except on Modal: a Modal sandbox only has a workspace when it was created with --sync.
--no-install copies the repository and skips the toolchain and the dependency commands, then prints those commands to run them by hand.
If <repo> already exists in the sandbox, sync stops before copying and does not replace it. A failed copy or install keeps the sandbox.`,
		Example: `  kaxa sync my-sprite
  kaxa sync --dir ~/projects/api-python my-sprite
  kaxa sync --no-install --dir ~/projects/api-go my-box
  kaxa --provider modal sync my-box`,
		Args: requireSyncName,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSync(cmd, reg, args[0], dir, cmd.Flags().Changed("dir"), noInstall)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Git repository to copy (default: the current directory)")
	cmd.Flags().BoolVar(&noInstall, "no-install", false, "copy the repository and skip the toolchain and dependency installs")
	return cmd
}

func runSync(cmd *cobra.Command, reg *registry.Registry, name, dir string, dirSet, noInstall bool) error {
	root, repo, err := resolveSyncRepo(dir, dirSet)
	if err != nil {
		return err
	}
	p, err := providerFromCmd(cmd, reg)
	if err != nil {
		return err
	}
	errOut := cmd.ErrOrStderr()
	res, err := lifecycle.Sync(cmd.Context(), p, lifecycle.SyncRequest{Name: name, Root: root, Repo: repo, NoInstall: noInstall}, func(ctx context.Context, label string, task func(context.Context) error) error {
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
	msg := fmt.Sprintf("%s copied to sandbox %s", repo, ref)
	if res.WorkDir != "" {
		msg = fmt.Sprintf("%s copied to sandbox %s at %s", repo, ref, res.WorkDir)
	}
	if len(res.Skipped) == 0 {
		return msg
	}
	var b strings.Builder
	b.WriteString(msg)
	b.WriteString("\n\ndependencies were not installed; run them with:")
	for _, argv := range res.Skipped {
		fmt.Fprintf(&b, "\n  kaxa exec %s -- sh -c 'cd %s && %s'", res.ID, skippedDir(res.WorkDir), strings.Join(argv, " "))
	}
	return b.String()
}

// skippedDir is where the skipped commands run.
func skippedDir(workDir string) string {
	if workDir == "" {
		return "$HOME/workspace/<repo>"
	}
	return workDir
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
