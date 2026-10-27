package cli

import (
	"errors"
	"io"

	"charm.land/huh/v2"

	"github.com/perylemke/sbox/internal/sandbox"
)

var errURLAuthCancelled = errors.New("url auth cancelled")

func collectURLAuth(in io.Reader, out io.Writer, current string) (string, error) {
	choice := selectedURLAuth(current)
	form := newThemedForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("URL auth").
				Options(
					huh.NewOption(sandbox.URLAuthPublic, sandbox.URLAuthPublic),
					huh.NewOption(sandbox.URLAuthSprite, sandbox.URLAuthSprite),
				).
				Value(&choice),
		).Title("Sandbox"),
	)
	if !inputIsTTY(in) {
		form = form.WithAccessible(true).WithInput(in).WithOutput(out)
	}
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", errURLAuthCancelled
		}
		return "", err
	}
	return choice, nil
}
