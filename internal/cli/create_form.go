package cli

import (
	"bufio"
	"errors"
	"io"
	"strconv"
	"strings"

	"charm.land/huh/v2"

	"github.com/perylemke/sbox/internal/usererr"
)

var errCreateCancelled = errors.New("create cancelled")

type createFormResult struct {
	Name     string
	Provider string
	Running  bool
	Sync     bool
	API      bool
	Public   bool
	APIPort  int
	APICmd   []string
}

func collectCreateForm(in io.Reader, out io.Writer, providers []string, defaultProvider string, flags createFlags) (createFormResult, error) {
	if len(providers) == 0 {
		return createFormResult{}, usererr.New("no sandbox providers registered", "check sbox installation")
	}

	var result createFormResult
	if defaultProvider != "" {
		result.Provider = defaultProvider
	}
	result.Running = flags.Running
	result.Sync = flags.Sync
	result.API = flags.API
	result.Public = flags.Public

	commandRaw := flags.APICmd
	portRaw := strconv.Itoa(flags.APIPort)
	if flags.APIPort == 0 {
		portRaw = strconv.Itoa(defaultAPIPort)
	}

	options := make([]huh.Option[string], 0, len(providers))
	for _, name := range providers {
		options = append(options, huh.NewOption(name, name))
	}

	main := huh.NewGroup(
		huh.NewInput().Title("Name").Value(&result.Name).Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return errors.New("name is required")
			}
			return nil
		}),
		huh.NewSelect[string]().Title("Provider").Options(options...).Value(&result.Provider),
		huh.NewConfirm().Title("Running").Affirmative("Yes").Negative("No").Value(&result.Running),
		huh.NewConfirm().Title("Sync").Affirmative("Yes").Negative("No").Value(&result.Sync),
		huh.NewConfirm().Title("API").Affirmative("Yes").Negative("No").Value(&result.API),
		huh.NewConfirm().Title("Public").Affirmative("Yes").Negative("No").Value(&result.Public),
	).Title("Create sandbox")
	detail := huh.NewGroup(
		huh.NewInput().Title("API command").Description("Comma-separated argv. The first item is the binary.").Value(&commandRaw).Validate(func(s string) error {
			_, err := parseAPICommand(s)
			if err != nil {
				return errors.New(err.Error())
			}
			return nil
		}),
		huh.NewInput().Title("API port").Value(&portRaw).Validate(func(s string) error {
			return validateAPIPortInput(s, result.Running)
		}),
	).Title("API")

	accessible := !inputIsTTY(in)
	reader := in
	if accessible {
		reader = newOneLineReader(in)
	}
	if err := runCreateForm(out, reader, accessible, main, detail, &result); err != nil {
		return createFormResult{}, err
	}
	result.Name = strings.TrimSpace(result.Name)
	if !result.API {
		return result, nil
	}
	argv, err := parseAPICommand(commandRaw)
	if err != nil {
		return createFormResult{}, err
	}
	port, err := parsedAPIPort(portRaw, result.Running)
	if err != nil {
		return createFormResult{}, err
	}
	result.APICmd = argv
	result.APIPort = port
	return result, nil
}

func runCreateForm(out io.Writer, in io.Reader, accessible bool, main, detail *huh.Group, result *createFormResult) error {
	var form *huh.Form
	if accessible {
		form = newThemedForm(main).WithAccessible(true).WithInput(in).WithOutput(out)
	} else {
		detail = detail.WithHideFunc(func() bool { return !result.API })
		form = newThemedForm(main, detail)
	}
	if err := form.Run(); err != nil {
		return formRunErr(err)
	}
	if !accessible || !result.API {
		return nil
	}
	next := newThemedForm(detail).WithAccessible(true).WithInput(in).WithOutput(out)
	if err := next.Run(); err != nil {
		return formRunErr(err)
	}
	return nil
}

func formRunErr(err error) error {
	if errors.Is(err, huh.ErrUserAborted) {
		return errCreateCancelled
	}
	return err
}

func validateAPIPortInput(raw string, running bool) error {
	_, err := parsedAPIPort(raw, running)
	return err
}

func parsedAPIPort(raw string, running bool) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || port < 1 || port > 65535 {
		return 0, errors.New("use a TCP port from 1 to 65535")
	}
	if running && port == webtmuxHTTPPort {
		return 0, errors.New("port 8080 is used by the tmux service")
	}
	return port, nil
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
		if err != nil && !errors.Is(err, bufio.ErrBufferFull) {
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
