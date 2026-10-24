package sprites

import (
	"context"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/sbox/internal/sandbox"
)

// SpriteListEntry is one sandbox row from paginated ListSprites.
type SpriteListEntry struct {
	Name    string
	Status  string
	Labels  []string
	Version string
}

func (r *realSDKClient) ListEntries(ctx context.Context, prefix string) ([]SpriteListEntry, error) {
	var entries []SpriteListEntry
	continuationToken := ""

	for {
		opts := &spritesdk.ListOptions{
			Prefix:            prefix,
			MaxResults:        100,
			ContinuationToken: continuationToken,
		}
		list, err := r.c.ListSprites(ctx, opts)
		if err != nil {
			return nil, err
		}
		for _, info := range list.Sprites {
			entries = append(entries, SpriteListEntry{
				Name:    info.Name,
				Status:  info.Status,
				Labels:  append([]string(nil), info.Labels...),
				Version: r.spriteVersion(ctx, info.Name),
			})
		}
		if !list.HasMore || list.NextContinuationToken == "" {
			break
		}
		continuationToken = list.NextContinuationToken
	}

	return entries, nil
}

// spriteVersion reads the per-sprite Sprite-Version header.
// The list API does not include that value; FetchVersion routes to the sprite.
func (r *realSDKClient) spriteVersion(ctx context.Context, name string) string {
	if err := r.c.FetchVersion(ctx, name); err != nil {
		return ""
	}
	return r.c.SpriteVersion()
}

func entriesToSandboxInfo(entries []SpriteListEntry, provider string) []sandbox.Info {
	out := make([]sandbox.Info, 0, len(entries))
	for _, e := range entries {
		out = append(out, sandbox.Info{
			ID:       sandbox.ID(e.Name),
			Provider: provider,
			Status:   e.Status,
			Version:  e.Version,
			Labels:   append([]string(nil), e.Labels...),
		})
	}
	return out
}
