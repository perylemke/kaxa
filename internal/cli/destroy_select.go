package cli

import (
	"io"
	"os"

	"golang.org/x/term"
)

func inputIsTTY(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
