package sprites

import (
	"context"
	"strings"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/sbox/internal/sandbox"
)

const defaultAPIBase = "https://api.sprites.dev"

func spriteToDetail(sp *spritesdk.Sprite, provider string) sandbox.Detail {
	return sandbox.Detail{
		ID:           sandbox.ID(sp.Name()),
		Provider:     provider,
		Status:       sp.Status,
		Labels:       append([]string(nil), sp.Labels...),
		SpriteUUID:   sp.ID,
		Organization: sp.OrganizationName,
		URL:          sp.URL,
		Auth:         formatSpriteAuth(sp.URLSettings),
		CreatedAt:    sp.CreatedAt,
		UpdatedAt:    sp.UpdatedAt,
	}
}

func formatSpriteAuth(settings *spritesdk.URLSettings) string {
	if settings == nil {
		return ""
	}
	auth := settings.Auth
	access := settings.PrivateAccess
	if auth == "" && access == "" {
		return ""
	}
	if auth != "" && access != "" {
		return auth + " · " + access
	}
	return strings.TrimSpace(auth + access)
}

func (p *Provider) Inspect(ctx context.Context, id sandbox.ID) (sandbox.Detail, error) {
	if err := validateName(string(id)); err != nil {
		return sandbox.Detail{}, err
	}
	c, token, err := p.clientAndToken()
	if err != nil {
		return sandbox.Detail{}, err
	}
	sp, err := c.GetSprite(ctx, string(id))
	if err != nil {
		return sandbox.Detail{}, redactErr(err, token)
	}
	return spriteToDetail(sp, p.Name()), nil
}
