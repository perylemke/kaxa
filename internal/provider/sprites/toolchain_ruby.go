package sprites

import "context"

// installRuby unpacks a relocatable Ruby. The bundle launcher resolves ruby next
// to itself, so ruby, gem, and bundle are linked side by side in the user bin.
func installRuby(ctx context.Context, c sdkClient, name, token string, arch spriteArch) error {
	asset := rubyAsset(arch)
	if err := downloadVerified(ctx, c, name, token, asset); err != nil {
		return installFailed("ruby", err)
	}
	if err := extractStrip(ctx, c, name, token, asset.archive, rubyPrefix); err != nil {
		return installFailed("ruby", err)
	}
	for _, tool := range []string{"ruby", "gem", "bundle"} {
		if err := linkTool(ctx, c, name, token, rubyPrefix+"/bin/"+tool, localBin+"/"+tool); err != nil {
			return installFailed("ruby", err)
		}
	}
	return nil
}

func linkTool(ctx context.Context, c sdkClient, name, token, target, link string) error {
	_, err := runRemote(ctx, c, name, token, []string{"ln", "-sfn", target, link})
	return err
}
