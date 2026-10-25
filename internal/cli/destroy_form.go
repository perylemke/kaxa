package cli

import (
	"errors"
	"fmt"
	"io"

	"charm.land/huh/v2"

	"github.com/perylemke/sbox/internal/sandbox"
)

func newDestroyForm(infos []sandbox.Info, selected *[]sandbox.ID) *huh.Form {
	options := make([]huh.Option[sandbox.ID], 0, len(infos))
	for _, info := range infos {
		label := fmt.Sprintf("%s  %s", info.ID, info.Status)
		options = append(options, huh.NewOption(label, info.ID))
	}

	// huh sizes an unset multi-select from the option list, then subtracts the
	// title. That leaves the option viewport at height 0, so the list is blank.
	return newThemedForm(
		huh.NewGroup(
			huh.NewMultiSelect[sandbox.ID]().
				Title("Sandboxes").
				Options(options...).
				Height(len(infos) + 1).
				Value(selected),
		).Title("Destroy sandboxes"),
	)
}

func collectDestroyForm(in io.Reader, out io.Writer, infos []sandbox.Info) ([]sandbox.ID, error) {
	var selected []sandbox.ID
	form := newDestroyForm(infos, &selected)
	if !inputIsTTY(in) {
		form = form.WithAccessible(true).WithInput(newOneLineReader(in)).WithOutput(out)
	}

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, errDestroyCancelled
		}
		return nil, err
	}
	if len(selected) == 0 {
		return nil, errDestroyCancelled
	}
	return selected, nil
}
