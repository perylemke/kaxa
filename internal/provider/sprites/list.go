package sprites

import (
	"context"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/sbox/internal/sandbox"
)

// SpriteListEntry is one sandbox row from paginated ListSprites.
type SpriteListEntry struct {
	Name   string
	Status string
	Labels []string
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
				Name:   info.Name,
				Status: info.Status,
				Labels: append([]string(nil), info.Labels...),
			})
		}
		if !list.HasMore || list.NextContinuationToken == "" {
			break
		}
		continuationToken = list.NextContinuationToken
	}

	return entries, nil
}

func entriesToSandboxInfo(entries []SpriteListEntry, provider string) []sandbox.Info {
	out := make([]sandbox.Info, 0, len(entries))
	for _, e := range entries {
		out = append(out, sandbox.Info{
			ID:       sandbox.ID(e.Name),
			Provider: provider,
			Status:   e.Status,
			Labels:   append([]string(nil), e.Labels...),
		})
	}
	return out
}
