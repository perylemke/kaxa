package toolchain

import "context"

// installRuby unpacks a relocatable Ruby. The bundle launcher resolves ruby next
// to itself, so ruby, gem, and bundle are linked side by side in the user bin.
func (i *installer) installRuby(ctx context.Context, arch archInfo) error {
	asset := rubyAsset(arch)
	if err := i.downloadVerified(ctx, asset); err != nil {
		return installFailed("ruby", err)
	}
	if err := i.extractStrip(ctx, asset.archive, i.l.rubyPrefix()); err != nil {
		return installFailed("ruby", err)
	}
	for _, tool := range []string{"ruby", "gem", "bundle"} {
		if err := i.linkTool(ctx, i.l.rubyPrefix()+"/bin/"+tool, i.l.localBin()+"/"+tool); err != nil {
			return installFailed("ruby", err)
		}
	}
	return nil
}

func (i *installer) linkTool(ctx context.Context, target, link string) error {
	_, err := i.run(ctx, []string{"ln", "-sfn", target, link})
	return err
}
