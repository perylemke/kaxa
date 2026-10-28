package cli

import (
	"errors"
	"fmt"
	"io"

	"charm.land/huh/v2"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// newDestroyForm offers every sandbox as "name  status  provider". Options are
// keyed by index because two providers can hold a sandbox with the same name.
func newDestroyForm(infos []sandbox.Info, selected *[]int) *huh.Form {
	options := make([]huh.Option[int], 0, len(infos))
	for i, info := range infos {
		label := fmt.Sprintf("%s  %s  %s", info.ID, info.Status, info.Provider)
		options = append(options, huh.NewOption(label, i))
	}

	// huh sizes an unset multi-select from the option list, then subtracts the
	// title. That leaves the option viewport at height 0, so the list is blank.
	return newThemedForm(
		huh.NewGroup(
			huh.NewMultiSelect[int]().
				Title("Sandboxes").
				Options(options...).
				Height(len(infos) + 1).
				Value(selected),
		).Title("Destroy sandboxes"),
	)
}

func collectDestroyForm(in io.Reader, out io.Writer, infos []sandbox.Info) ([]sandbox.Info, error) {
	var selected []int
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
	chosen := make([]sandbox.Info, 0, len(selected))
	for _, i := range selected {
		chosen = append(chosen, infos[i])
	}
	return chosen, nil
}
