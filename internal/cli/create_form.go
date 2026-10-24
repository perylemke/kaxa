package cli

import (
	"bufio"
	"errors"
	"io"
	"strings"

	"charm.land/huh/v2"

	"github.com/perylemke/sbox/internal/usererr"
)

var errCreateCancelled = errors.New("create cancelled")

type createFormResult struct {
	Name     string
	Provider string
	Running  bool
}

func collectCreateForm(in io.Reader, out io.Writer, providers []string, defaultProvider string, flagRunning bool) (createFormResult, error) {
	if len(providers) == 0 {
		return createFormResult{}, usererr.New("no sandbox providers registered", "check sbox installation")
	}

	var result createFormResult
	if defaultProvider != "" {
		result.Provider = defaultProvider
	}
	result.Running = flagRunning

	options := make([]huh.Option[string], 0, len(providers))
	for _, name := range providers {
		options = append(options, huh.NewOption(name, name))
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Name").
				Value(&result.Name).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return errors.New("name is required")
					}
					return nil
				}),
			huh.NewSelect[string]().
				Title("Provider").
				Options(options...).
				Value(&result.Provider),
			huh.NewConfirm().
				Title("Running").
				Affirmative("Yes").
				Negative("No").
				Value(&result.Running),
		).Title("Create sandbox"),
	)
	if !inputIsTTY(in) {
		form = form.WithAccessible(true).WithInput(newOneLineReader(in)).WithOutput(out)
	}

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return createFormResult{}, errCreateCancelled
		}
		return createFormResult{}, err
	}
	result.Name = strings.TrimSpace(result.Name)
	return result, nil
}

// oneLineReader returns at most one line per Read so huh's accessible prompts,
// which each create a new buffered scanner, do not consume later answers.
type oneLineReader struct {
	r    *bufio.Reader
	rest []byte
}

func newOneLineReader(r io.Reader) *oneLineReader {
	return &oneLineReader{r: bufio.NewReader(r)}
}

func (o *oneLineReader) Read(p []byte) (int, error) {
	if len(o.rest) == 0 {
		line, err := o.r.ReadSlice('\n')
		if len(line) == 0 {
			return 0, err
		}
		o.rest = append([]byte(nil), line...)
		if err != nil && err != bufio.ErrBufferFull {
			n := copy(p, o.rest)
			o.rest = o.rest[n:]
			if len(o.rest) == 0 {
				return n, err
			}
			return n, nil
		}
	}
	n := copy(p, o.rest)
	o.rest = o.rest[n:]
	return n, nil
}
