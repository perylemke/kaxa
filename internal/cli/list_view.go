package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"golang.org/x/term"

	"github.com/perylemke/kaxa/internal/sandbox"
)

const (
	listColName     = "Name"
	listColStatus   = "Status"
	listColProvider = "Provider"
	listColLabels   = "Labels"
)

type listModel struct {
	table table.Model
}

func newListTable(infos []sandbox.Info, focused bool, height int) table.Model {
	rows := make([]table.Row, 0, len(infos))
	for _, info := range infos {
		rows = append(rows, table.Row{
			string(info.ID),
			info.Status,
			info.Provider,
			joinListLabels(info.Labels),
		})
	}

	columns := []table.Column{
		{Title: listColName, Width: 24},
		{Title: listColStatus, Width: 12},
		{Title: listColProvider, Width: 16},
		{Title: listColLabels, Width: 32},
	}

	if height < 2 {
		height = max(2, len(rows)+2)
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(focused),
		table.WithHeight(height),
	)
	s := table.DefaultStyles()
	s.Header = s.Header.Bold(true)
	t.SetStyles(s)

	return t
}

func newListModel(infos []sandbox.Info) listModel {
	tableHeight := max(2, len(infos)+2)
	return listModel{table: newListTable(infos, false, tableHeight)}
}

func (m listModel) View() string {
	return m.table.View() + "\n"
}

func writeListPlain(w io.Writer, infos []sandbox.Info) error {
	header := strings.Join([]string{"name", "status", "provider", "labels"}, "\t")
	if _, err := fmt.Fprintln(w, header); err != nil {
		return err
	}
	for _, info := range infos {
		labels := joinListLabels(info.Labels)
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", info.ID, info.Status, info.Provider, labels); err != nil {
			return err
		}
	}
	return nil
}

func joinListLabels(labels []string) string {
	return strings.Join(labels, ",")
}

func listOutputIsTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
