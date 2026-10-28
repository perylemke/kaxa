package modal

import (
	"context"
	"errors"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// drainGrace bounds how long Exec waits for output after the command exited,
// so a stream that never reports EOF cannot hang the CLI.
const drainGrace = 5 * time.Second

// sttyWrapper sets the terminal size, then replaces itself with the command.
// The size and the command are positional parameters, never part of the script,
// so nothing the user types is parsed by the shell. A failing stty is ignored:
// the command still runs, only with Modal's default 80x24 terminal.
const sttyWrapper = `stty rows "$1" cols "$2" 2>/dev/null; shift 2; exec "$@"`

// Exec runs a command in a running sandbox and returns when it exits. A non-zero
// exit code is returned as *sandbox.ExitError.
//
// A sandbox created with --sync runs the command as the modal user in
// /home/modal/workspace; any other sandbox runs it as root in the image's
// default directory.
//
// With TTY, Modal merges stderr into stdout, and the terminal size is applied once
// at start: the Modal SDK has no way to resize a running terminal, so opts.Resize
// is ignored.
func (p *Provider) Exec(ctx context.Context, id sandbox.ID, opts sandbox.ExecOptions) error {
	name := string(id)
	if err := validateName(name); err != nil {
		return err
	}
	if len(opts.Command) == 0 {
		return errors.New("modal exec: empty command")
	}
	c, secrets, err := p.clientAndSecrets()
	if err != nil {
		return err
	}
	// Modal still resolves a finished sandbox by name, so check it is running.
	rec, err := c.Get(ctx, name)
	if err != nil {
		return userErr(err, name, secrets...)
	}
	if rec.ExitCode != nil {
		return userErr(ErrNotFound, name, secrets...)
	}
	if isSynced(rec) {
		opts.Command = asModal(nil, opts.Command)
		return p.run(ctx, id, workspaceDir, opts)
	}
	return p.run(ctx, id, "", opts)
}

// run starts opts.Command in dir and waits for it. The caller has checked the
// sandbox and chosen the user.
func (p *Provider) run(ctx context.Context, id sandbox.ID, dir string, opts sandbox.ExecOptions) error {
	name := string(id)
	c, secrets, err := p.clientAndSecrets()
	if err != nil {
		return err
	}
	proc, err := c.Exec(ctx, name, execArgv(opts), ExecParams{PTY: opts.TTY, Workdir: dir})
	if err != nil {
		return userErr(err, name, secrets...)
	}
	defer proc.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	discardResize(ctx, opts.Resize)

	pumps := startPumps(proc, opts)
	code, err := proc.Wait(ctx)
	if err != nil {
		return userErr(err, name, secrets...)
	}
	copyErr := pumps.wait()
	if code != 0 {
		return &sandbox.ExitError{Code: code}
	}
	return userErr(copyErr, name, secrets...)
}

func execArgv(opts sandbox.ExecOptions) []string {
	if !opts.TTY || opts.Size.Rows == 0 || opts.Size.Cols == 0 {
		return opts.Command
	}
	argv := []string{
		"sh", "-c", sttyWrapper, "kaxa",
		strconv.Itoa(int(opts.Size.Rows)), strconv.Itoa(int(opts.Size.Cols)),
	}
	return append(argv, opts.Command...)
}

// discardResize drains resize events so the sender never blocks.
func discardResize(ctx context.Context, resize <-chan sandbox.TermSize) {
	if resize == nil {
		return
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-resize:
				if !ok {
					return
				}
			}
		}
	}()
}

type pumps struct {
	wg   sync.WaitGroup
	mu   sync.Mutex
	err  error
	done chan struct{}
}

// startPumps copies stdin to the process and its output to opts. The stdin pump
// is not awaited: it may be blocked reading a terminal.
func startPumps(proc Process, opts sandbox.ExecOptions) *pumps {
	p := &pumps{done: make(chan struct{})}

	go func() {
		defer func() { _ = proc.Stdin().Close() }()
		if opts.Stdin != nil {
			_, _ = io.Copy(proc.Stdin(), opts.Stdin)
		}
	}()

	p.copy(opts.Stdout, proc.Stdout())
	if !opts.TTY {
		p.copy(opts.Stderr, proc.Stderr())
	}
	go func() {
		p.wg.Wait()
		close(p.done)
	}()
	return p
}

func (p *pumps) copy(dst io.Writer, src io.Reader) {
	if dst == nil {
		dst = io.Discard
	}
	p.wg.Go(func() {
		if _, err := io.Copy(dst, src); err != nil {
			p.mu.Lock()
			p.err = errors.Join(p.err, err)
			p.mu.Unlock()
		}
	})
}

// wait returns once the output is drained or the grace period ends.
func (p *pumps) wait() error {
	select {
	case <-p.done:
	case <-time.After(drainGrace):
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}
