package sprites

import (
	"context"
	"strings"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/kaxa/internal/sandbox"
)

const defaultAPIBase = "https://api.sprites.dev"

func spriteToDetail(sp *spritesdk.Sprite, provider string) sandbox.Detail {
	auth := formatSpriteAuth(sp.URLSettings)
	d := sandbox.Detail{
		ID:           sandbox.ID(sp.Name()),
		Provider:     provider,
		Status:       sp.Status,
		Labels:       append([]string(nil), sp.Labels...),
		SpriteUUID:   sp.ID,
		Organization: sp.OrganizationName,
		URL:          sp.URL,
		Auth:         auth,
		CreatedAt:    sp.CreatedAt,
		UpdatedAt:    sp.UpdatedAt,
	}
	d.Fields = spriteDetailFields(sp, auth)
	return d
}

func spriteDetailFields(sp *spritesdk.Sprite, auth string) []sandbox.Field {
	var fields []sandbox.Field
	if sp.ID != "" {
		fields = sandbox.AppendTextField(fields, "ID", "sprite-"+sp.ID)
	}
	fields = sandbox.AppendTextField(fields, "Organization", sp.OrganizationName)
	fields = sandbox.AppendTextField(fields, "Region", sp.PrimaryRegion)
	if sp.Config != nil {
		fields = sandbox.AppendIntField(fields, "CPUs", sp.Config.CPUs)
		if sp.Config.RamMB > 0 {
			fields = sandbox.AppendFormattedField(fields, "Memory", "%d MB", sp.Config.RamMB)
		}
		if sp.Config.StorageGB > 0 {
			fields = sandbox.AppendFormattedField(fields, "Storage", "%d GB", sp.Config.StorageGB)
		}
	}
	fields = sandbox.AppendTextField(fields, "URL", sp.URL)
	fields = sandbox.AppendTextField(fields, "Auth", auth)
	fields = sandbox.AppendTimeField(fields, "Created", sp.CreatedAt)
	fields = sandbox.AppendTimeField(fields, "Updated", sp.UpdatedAt)
	fields = sandbox.AppendOptionalTimeField(fields, "Last running", sp.LastRunningAt)
	fields = sandbox.AppendOptionalTimeField(fields, "Last warming", sp.LastWarmingAt)
	return fields
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

// Inspect returns the full sprite record.
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
