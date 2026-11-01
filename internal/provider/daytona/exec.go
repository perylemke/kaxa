package daytona

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
)

const (
	inputChunk  = 32 << 10
	defaultRows = 24
	defaultCols = 80
	termName    = "xterm-256color"
	// waitWindow is how long one long-poll of a running command lasts. The poll
	// returns at once when the command ends.
	waitWindow = 750 * time.Millisecond
)

// session is a sandbox that is running and ready for commands.
type session struct {
	tb      toolboxClient
	conn    Conn
	rec     Record
	secrets []string
}

// open finds the sandbox and starts it if Daytona stopped it.
func (p *Provider) open(ctx context.Context, id sandbox.ID) (*session, error) {
	if err := validateName(string(id)); err != nil {
		return nil, err
	}
	c, tb, secrets, err := p.clients()
	if err != nil {
		return nil, err
	}
	rec, err := c.Find(ctx, string(id))
	if err != nil {
		return nil, userErr(err, string(id), secrets...)
	}
	rec, err = p.ensureRunning(ctx, c, rec)
	if err != nil {
		return nil, userErr(err, string(id), secrets...)
	}
	return &session{
		tb: tb, conn: Conn{SandboxID: rec.ID, BaseURL: rec.ToolboxURL}, rec: rec, secrets: secrets,
	}, nil
}

func (s *session) err(err error) error {
	return userErr(err, s.rec.Name, s.secrets...)
}

func (s *session) synced() bool {
	return slices.Contains(s.rec.Labels, sandbox.SyncLabel)
}

// runSpec is one command of a session.
type runSpec struct {
	argv []string
	dir  string
	// env is KEY=VALUE entries added to the environment. The command is
	// looked up in the PATH they set.
	env    []string
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	tty    bool
	size   sandbox.TermSize
	resize <-chan sandbox.TermSize
}

// Exec runs a command in a sandbox and returns when it exits. A non-zero exit
// code is returned as *sandbox.ExitError.
//
// A sandbox created with --sync runs the command in /home/daytona/workspace
// with the installed toolchains on PATH; any other runs it in the home
// directory. Daytona starts a stopped sandbox first.
func (p *Provider) Exec(ctx context.Context, id sandbox.ID, opts sandbox.ExecOptions) error {
	if len(opts.Command) == 0 {
		return errors.New("daytona exec: empty command")
	}
	s, err := p.open(ctx, id)
	if err != nil {
		return err
	}
	spec := runSpec{
		argv: opts.Command, stdin: opts.Stdin, stdout: opts.Stdout, stderr: opts.Stderr,
		tty: opts.TTY, size: opts.Size, resize: opts.Resize,
	}
	if s.synced() {
		spec.dir, spec.env = workspaceDir, daytonaLayout.Env()
	}
	return s.run(ctx, spec)
}

func (s *session) run(ctx context.Context, spec runSpec) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if spec.tty {
		return s.runPTY(ctx, spec)
	}
	return s.runExec(ctx, spec)
}

// commandOf builds the argv sent to the toolbox. env(1) sets the variables and
// then looks the command up, so tools installed under the home directory are
// found. The toolbox is given an argv, so nothing is parsed by a shell.
func commandOf(argv, env []string) []string {
	if len(env) == 0 {
		return slices.Clone(argv)
	}
	return append(append([]string{"env"}, env...), argv...)
}

func termSize(size sandbox.TermSize) (rows, cols uint16) {
	if size.Rows == 0 || size.Cols == 0 {
		return defaultRows, defaultCols
	}
	return size.Rows, size.Cols
}

// logPuller copies the log of one process, after a cursor, to the writers of a run.
type logPuller struct {
	s       *session
	spec    runSpec
	id      string
	cursor  string
	copyErr error
}

// pull copies the output after the cursor and reports whether there was any.
func (l *logPuller) pull(ctx context.Context) (bool, error) {
	frames, err := l.s.tb.Logs(ctx, l.s.conn, l.id, l.cursor)
	if err != nil {
		return false, l.s.err(err)
	}
	last := l.cursor
	for _, f := range frames {
		if f.Cursor != "" {
			last = f.Cursor
		}
		if werr := writeFrame(l.spec, f); werr != nil && l.copyErr == nil {
			l.copyErr = werr
		}
	}
	progressed := len(frames) > 0 && last != l.cursor
	l.cursor = last
	return progressed, nil
}

// drain pulls until the log has nothing more.
func (l *logPuller) drain(ctx context.Context) error {
	for {
		more, err := l.pull(ctx)
		if err != nil || !more {
			return err
		}
	}
}

func (s *session) runExec(ctx context.Context, spec runSpec) error {
	id, err := s.tb.Start(ctx, s.conn, ProcessRequest{
		Kind: kindExec, Argv: commandOf(spec.argv, spec.env), Dir: spec.dir, Stdin: spec.stdin != nil,
	})
	if err != nil {
		return s.err(err)
	}
	if spec.stdin != nil {
		go s.pumpStdin(ctx, id, spec.stdin)
	}
	logs := &logPuller{s: s, spec: spec, id: id}
	for {
		more, err := logs.pull(ctx)
		if err != nil {
			return err
		}
		if more {
			continue
		}
		res, err := s.tb.Wait(ctx, s.conn, id, waitWindow)
		if err != nil {
			return s.err(err)
		}
		if !res.Done() {
			continue
		}
		// Output that arrived with the exit is read after it.
		if err := logs.drain(ctx); err != nil {
			return err
		}
		return s.exitOf(res, logs.copyErr)
	}
}

// writeFrame sends a frame to the writer of its channel. Channels other than
// stdout and stderr, such as setup, are progress notes and are dropped.
func writeFrame(spec runSpec, f LogFrame) error {
	var w io.Writer
	switch f.Channel {
	case "stdout", "":
		w = spec.stdout
	case "stderr":
		w = spec.stderr
	}
	if w == nil {
		return nil
	}
	_, err := io.WriteString(w, f.Data)
	return err
}

// pumpStdin copies in to the command and closes its standard input at the end.
// It is not awaited: it may be blocked reading when the command ends.
func (s *session) pumpStdin(ctx context.Context, id string, in io.Reader) {
	buf := make([]byte, inputChunk)
	for {
		n, readErr := in.Read(buf)
		eof := readErr != nil
		if n > 0 || eof {
			if err := s.tb.Stdin(ctx, s.conn, id, slices.Clone(buf[:n]), eof); err != nil {
				return
			}
		}
		if eof {
			return
		}
	}
}

func (s *session) runPTY(ctx context.Context, spec runSpec) error {
	rows, cols := termSize(spec.size)
	env := append([]string{"TERM=" + termName}, spec.env...)
	id, err := s.tb.Start(ctx, s.conn, ProcessRequest{
		Kind: kindPTY, Argv: commandOf(spec.argv, env), Dir: spec.dir, Stdin: true, Rows: rows, Cols: cols,
	})
	if err != nil {
		return s.err(err)
	}
	att, err := s.tb.Attach(ctx, s.conn, id)
	if err != nil {
		return s.err(err)
	}
	defer func() { _ = att.Close() }()
	go pumpTerminal(att, spec.stdin)
	go s.watchResize(ctx, id, spec.resize)

	var copyErr error
	for {
		data, err := att.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return s.err(err)
		}
		if spec.stdout == nil {
			continue
		}
		if _, werr := spec.stdout.Write(data); werr != nil && copyErr == nil {
			copyErr = werr
		}
	}
	res, err := s.tb.Wait(ctx, s.conn, id, 0)
	if err != nil {
		return s.err(err)
	}
	return s.exitOf(res, copyErr)
}

// pumpTerminal copies in to the terminal. It is not awaited: it may be blocked
// reading a terminal when the command ends. A terminal has no end of input.
func pumpTerminal(att Attachment, in io.Reader) {
	if in == nil {
		return
	}
	buf := make([]byte, inputChunk)
	for {
		n, readErr := in.Read(buf)
		if n > 0 {
			if err := att.Write(slices.Clone(buf[:n])); err != nil {
				return
			}
		}
		if readErr != nil {
			return
		}
	}
}

func (s *session) watchResize(ctx context.Context, id string, resize <-chan sandbox.TermSize) {
	if resize == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case size, ok := <-resize:
			if !ok {
				return
			}
			if size.Rows > 0 && size.Cols > 0 {
				_ = s.tb.Resize(ctx, s.conn, id, size.Rows, size.Cols)
			}
		}
	}
}

func (s *session) exitOf(res WaitResult, copyErr error) error {
	switch {
	case res.Reason == reasonExited && res.HasCode && res.ExitCode != 0:
		return &sandbox.ExitError{Code: res.ExitCode}
	case res.Reason == reasonSignaled:
		// A command stopped by a signal has no exit code.
		return &sandbox.ExitError{Code: 1}
	case res.Reason != reasonExited:
		return s.err(fmt.Errorf("command ended: %s", res.Reason))
	}
	return s.err(copyErr)
}

// capture runs argv in dir and returns its stdout. A failure carries the
// command's stderr, which the installer needs to explain what went wrong.
//
// A non-zero exit is returned as a plain error, not *sandbox.ExitError: the CLI
// turns an ExitError into the exit code of the process with no message, which
// is right for kaxa exec but would hide why a copy or install failed.
func (s *session) capture(ctx context.Context, dir string, env, argv []string, stdin io.Reader) (string, error) {
	stdout := sandbox.NewCappedBuffer(sandbox.MaxCapturedOutput)
	stderr := sandbox.NewCappedBuffer(sandbox.MaxCapturedOutput)
	err := s.run(ctx, runSpec{
		argv: argv, dir: dir, env: append(daytonaLayout.Env(), env...),
		stdin: stdin, stdout: stdout, stderr: stderr,
	})
	if err == nil {
		return stdout.String(), nil
	}
	detail := strings.TrimSpace(stderr.String())
	if exitErr, ok := errors.AsType[*sandbox.ExitError](err); ok {
		msg := exitErr.Error()
		if detail != "" {
			msg += ": " + detail
		}
		return "", errors.New(msg)
	}
	if detail != "" {
		return "", fmt.Errorf("%w: %s", err, detail)
	}
	return "", err
}
