package blaxel

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// ListTemplates returns the visible Hub images Blaxel offers as create
// templates, sorted by image. Hidden and coming-soon entries are left out.
// The option name is the image reference passed to create.
func (p *Provider) ListTemplates(ctx context.Context) ([]sandbox.TemplateOption, error) {
	c, _, secrets, err := p.clients()
	if err != nil {
		return nil, err
	}
	rows, err := c.ListHub(ctx)
	if err != nil {
		return nil, userErr(err, "", secrets...)
	}
	return templateOptions(rows), nil
}

func templateOptions(rows []hubTemplate) []sandbox.TemplateOption {
	out := make([]sandbox.TemplateOption, 0, len(rows))
	for _, row := range rows {
		if row.Hidden || row.ComingSoon || row.Image == "" {
			continue
		}
		out = append(out, sandbox.TemplateOption{
			Name:  row.Image,
			Label: templateLabel(row),
		})
	}
	slices.SortFunc(out, func(a, b sandbox.TemplateOption) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

func templateLabel(row hubTemplate) string {
	image := row.Image
	name := strings.TrimSpace(row.DisplayName)
	switch {
	case name != "" && row.Memory > 0:
		return fmt.Sprintf("%s (%s, %d MB)", name, image, row.Memory)
	case name != "":
		return fmt.Sprintf("%s (%s)", name, image)
	case row.Memory > 0:
		return fmt.Sprintf("%s (%d MB)", image, row.Memory)
	default:
		return image
	}
}
