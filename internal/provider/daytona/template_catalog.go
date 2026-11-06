package daytona

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// ListTemplates returns the active platform snapshots Daytona offers as
// create templates, sorted by name. Account snapshots and inactive ones are
// left out.
func (p *Provider) ListTemplates(ctx context.Context) ([]sandbox.TemplateOption, error) {
	c, _, secrets, err := p.clients()
	if err != nil {
		return nil, err
	}
	rows, err := c.ListSnapshotCatalog(ctx)
	if err != nil {
		return nil, userErr(err, "", secrets...)
	}
	return templateOptions(rows), nil
}

func templateOptions(rows []snapshotCatalogRow) []sandbox.TemplateOption {
	out := make([]sandbox.TemplateOption, 0, len(rows))
	for _, row := range rows {
		if !row.General || row.State != snapshotStateActive || row.Name == "" {
			continue
		}
		out = append(out, sandbox.TemplateOption{
			Name:  row.Name,
			Label: templateLabel(row),
		})
	}
	slices.SortFunc(out, func(a, b sandbox.TemplateOption) int {
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

func templateLabel(row snapshotCatalogRow) string {
	cores := "CPUs"
	if row.CPU == 1 {
		cores = "CPU"
	}
	return fmt.Sprintf("%s (%d %s, %d GB memory, %d GB disk)", row.Name, row.CPU, cores, row.Memory, row.Disk)
}
