package sprites

import (
	"strings"
	"testing"
	"time"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestSpriteDetailFields(t *testing.T) {
	t.Parallel()

	lastRun := time.Now().Add(-time.Hour)
	sp := &spritesdk.Sprite{
		ID:               "uuid-1",
		OrganizationName: "acme",
		Status:           "warm",
		PrimaryRegion:    "iad",
		Config:           &spritesdk.SpriteConfig{CPUs: 2, RamMB: 512, StorageGB: 10},
		URL:              "https://demo.sprites.dev",
		URLSettings:      &spritesdk.URLSettings{Auth: "sprite", PrivateAccess: "admins"},
		CreatedAt:        time.Now().Add(-2 * time.Hour),
		UpdatedAt:        time.Now().Add(-5 * time.Minute),
		LastRunningAt:    &lastRun,
	}
	d := spriteToDetail(sp, "sprites")
	if d.Organization != "acme" || d.URL == "" {
		t.Fatalf("detail = %+v", d)
	}
	text := fieldsText(d.Fields)
	for _, want := range []string{"sprite-uuid-1", "acme", "iad", "CPUs", "512 MB", "10 GB", "Last running"} {
		if !strings.Contains(text, want) {
			t.Fatalf("fields missing %q: %s", want, text)
		}
	}
	for _, mustNot := range []string{"bucket", "environment"} {
		if strings.Contains(strings.ToLower(text), mustNot) {
			t.Fatalf("fields must not mention %q: %s", mustNot, text)
		}
	}
}

func fieldsText(fields []sandbox.Field) string {
	var b strings.Builder
	for _, f := range fields {
		b.WriteString(f.Label)
		b.WriteByte(' ')
		if !f.At.IsZero() {
			b.WriteString(f.At.Format(time.RFC3339))
		} else {
			b.WriteString(f.Text)
		}
		b.WriteByte(';')
	}
	return b.String()
}
