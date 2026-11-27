package vercel

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// ListTemplates offers the manual snapshots of the project as templates for
// the create form, newest first. On Vercel a template is a snapshot ID. A
// comment kaxa recorded on this machine is shown next to its snapshot.
func (p *Provider) ListTemplates(ctx context.Context) ([]sandbox.TemplateOption, error) {
	c, secrets, err := p.clients()
	if err != nil {
		return nil, err
	}
	rows, err := c.ListSnapshots(ctx, "")
	if err != nil {
		return nil, userErr(err, "", secrets...)
	}
	rows = onlyManual(rows)
	slices.SortStableFunc(rows, func(a, b Snapshot) int { return b.CreatedAt.Compare(a.CreatedAt) })
	comments := p.snapshotComments()
	out := make([]sandbox.TemplateOption, 0, len(rows))
	for _, row := range rows {
		out = append(out, sandbox.TemplateOption{Name: row.ID, Label: templateLabel(row, comments[row.ID])})
	}
	return out, nil
}

// snapshotComments reads the local comment index. The comments are optional,
// so an index that cannot be read leaves the labels without them.
func (p *Provider) snapshotComments() map[string]string {
	idx, err := p.index()
	if err != nil {
		return nil
	}
	entries, err := idx.load()
	if err != nil {
		return nil
	}
	comments := make(map[string]string, len(entries))
	for _, e := range entries {
		comments[e.ID] = e.Comment
	}
	return comments
}

func templateLabel(row Snapshot, comment string) string {
	var details []string
	if !row.CreatedAt.IsZero() {
		details = append(details, row.CreatedAt.UTC().Format(time.DateOnly))
	}
	if comment != "" {
		details = append(details, comment)
	}
	if len(details) == 0 {
		return row.ID
	}
	return fmt.Sprintf("%s (%s)", row.ID, strings.Join(details, ", "))
}
