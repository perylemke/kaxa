package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"golang.org/x/term"

	"github.com/perylemke/sbox/internal/usererr"
)

// WriteError renders err to w as a styled block (plain text when w is not a color TTY).
func WriteError(w io.Writer, err error) error {
	if err == nil {
		return nil
	}

	message := err.Error()
	hint := ""
	if ue, ok := usererr.As(err); ok {
		message = ue.Message
		hint = ue.Hint
	}

	r := lipgloss.NewRenderer(w)
	if !colorEnabled(w) {
		r.SetColorProfile(termenv.Ascii)
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9"))
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))

	var body strings.Builder
	body.WriteString("  ")
	body.WriteString(message)
	if hint != "" {
		body.WriteString("\n\n  ")
		body.WriteString(hintStyle.Render("hint  " + hint))
	}

	border := lipgloss.RoundedBorder()
	border.TopLeft = "╭"
	border.TopRight = "╮"
	border.BottomLeft = "╰"
	border.BottomRight = "╯"

	title := titleStyle.Render("error")
	inner := title + "\n\n" + body.String()
	box := lipgloss.NewStyle().
		Border(border, true).
		BorderForeground(lipgloss.Color("9")).
		Padding(0, 1).
		Render(inner)

	_, writeErr := fmt.Fprintln(w, box)
	return writeErr
}

// WriteSuccess renders a confirmation after a command finishes.
func WriteSuccess(w io.Writer, message string) error {
	r := lipgloss.NewRenderer(w)
	if !colorEnabled(w) {
		r.SetColorProfile(termenv.Ascii)
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))
	border := lipgloss.RoundedBorder()
	border.TopLeft = "╭"
	border.TopRight = "╮"
	border.BottomLeft = "╰"
	border.BottomRight = "╯"

	inner := titleStyle.Render("created") + "\n\n  " + message
	box := lipgloss.NewStyle().
		Border(border, true).
		BorderForeground(lipgloss.Color("2")).
		Padding(0, 1).
		Render(inner)

	_, err := fmt.Fprintln(w, box)
	return err
}

func colorEnabled(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// stripANSI removes ANSI sequences for assertions in tests.
func stripANSI(s string) string {
	return ansi.Strip(s)
}
