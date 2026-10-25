package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/perylemke/sbox/internal/registry"
	"github.com/perylemke/sbox/internal/sandbox"
	"github.com/perylemke/sbox/internal/usererr"
)

func newURLCmd(reg *registry.Registry) *cobra.Command {
	return &cobra.Command{
		Use:   "url <name> <public|sprite>",
		Short: "Set Sprites URL authentication",
		Long: `Set a Sprites sandbox URL to public or sprite.

public lets anyone with the address open the URL. sprite requires a Sprites account. This command is only supported for the sprites provider.`,
		Example: `  sbox url my-sprite public
  sbox url my-sprite sprite`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runURL(cmd, reg, args[0], args[1])
		},
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
		return usererr.New(
			"url auth is only supported for the sprites provider",
			"pass --provider sprites",
		)
	}
	id := sandbox.ID(name)
	if err := setter.SetURLAuth(cmd.Context(), id, auth); err != nil {
		return err
	}
	detail, err := p.Inspect(cmd.Context(), id)
	if err != nil {
		return err
	}
	msg := fmt.Sprintf("sandbox %s URL auth is %s\n%s", id, auth, detail.URL)
	return WriteSuccess(cmd.OutOrStdout(), "url", msg)
}
