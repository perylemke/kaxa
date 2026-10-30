package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/perylemke/kaxa/internal/lifecycle"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

func newURLCmd(reg *registry.Registry) *cobra.Command {
	return &cobra.Command{
		Use:         "url <name> <public|sprite>",
		Short:       "Set Sprites URL authentication",
		Annotations: featureCommand(sandbox.FeatureURLAuth),
		Long: `Set a Sprites sandbox URL to public or sprite.

public lets anyone with the address open the URL. sprite requires a Sprites account. This command is only supported for the sprites provider.`,
		Example: `  kaxa url my-sprite public
  kaxa url my-sprite sprite`,
		Args: requireURLArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runURL(cmd, reg, args[0], args[1])
		},
	}
}

func requireURLArgs(_ *cobra.Command, args []string) error {
	const hint = "kaxa url <name> public|sprite"
	switch len(args) {
	case 2:
		return nil
	case 0:
		return usererr.New("url requires a sandbox name and an auth mode", hint)
	case 1:
		return usererr.New("url requires an auth mode", hint)
	default:
		return usererr.New("url accepts a sandbox name and an auth mode", hint)
	}
}

func runURL(cmd *cobra.Command, reg *registry.Registry, name, auth string) error {
	if auth != sandbox.URLAuthPublic && auth != sandbox.URLAuthSprite {
		return usererr.New(fmt.Sprintf("invalid url auth %q", auth), "use public or sprite")
	}
	p, err := providerFromCmd(cmd, reg)
	if err != nil {
		return err
	}
	setter, ok := p.(sandbox.PublicURL)
	if !ok {
		return lifecycle.Unsupported(p, sandbox.FeatureURLAuth)
	}
	id := sandbox.ID(name)
	if err := setter.SetURLAuth(cmd.Context(), id, auth); err != nil {
		return err
	}
	detail, err := p.Inspect(cmd.Context(), id)
	if err != nil {
		return err
	}
	msg := fmt.Sprintf("sandbox %s URL auth is %s\n%s", sandboxLabel(p.Name(), id), auth, detail.URL)
	return WriteSuccess(cmd.OutOrStdout(), "url", msg)
}
