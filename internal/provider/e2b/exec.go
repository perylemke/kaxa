package e2b

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/perylemke/kaxa/internal/sandbox"
)

const (
	inputChunk  = 32 << 10
	defaultRows = 24
	defaultCols = 80
	termName    = "xterm-256color"
)

// session is a sandbox that was resumed and is ready to run commands. It holds
// one daemon token for every command of one operation.
type session struct {
	envd    envdClient
	conn    Connection
	rec     Record
	secrets []string
}

// open finds the sandbox, resumes it if E2B paused it, and extends its timeout.
func (p *Provider) open(ctx context.Context, id sandbox.ID) (*session, error) {
	if err := validateName(string(id)); err != nil {
		return nil, err
	}
	c, envd, secrets, err := p.clients()
	if err != nil {
		return nil, err
	}
	rec, err := c.Find(ctx, string(id))
	if err != nil {
		return nil, userErr(err, string(id), secrets...)
	}
	conn, err := c.Connect(ctx, rec.ID, ConnectParams{})
	if err != nil {
		return nil, userErr(err, string(id), secrets...)
	}
	return &session{envd: envd, conn: conn, rec: rec, secrets: append(secrets, conn.AccessToken)}, nil
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
// A sandbox created with --sync runs the command in /home/user/workspace with
// the installed toolchains on PATH; any other runs it in the home directory.
// E2B resumes a paused sandbox first.
func (p *Provider) Exec(ctx context.Context, id sandbox.ID, opts sandbox.ExecOptions) error {
	if len(opts.Command) == 0 {
		return errors.New("e2b exec: empty command")
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
		spec.dir, spec.env = workspaceDir, e2bLayout.Env()
	}
	return s.run(ctx, spec)
}

func (s *session) run(ctx context.Context, spec runSpec) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd, args := commandOf(spec.argv, spec.env)
	req := StartRequest{
		Cmd: cmd, Args: args, Cwd: spec.dir, User: e2bUser,
		PTY: spec.tty, Stdin: spec.stdin != nil || spec.tty,
	}
	if spec.tty {
		req.Rows, req.Cols = termSize(spec.size)
		req.Env = map[string]string{"TERM": termName}
	}
	proc, err := s.envd.Start(ctx, s.conn, req)
	if err != nil {
		return s.err(err)
	}
	defer proc.Close()

	if req.Stdin {
		go pumpInput(ctx, proc, spec.stdin, spec.tty)
	}
	if spec.tty {
		go watchResize(ctx, proc, spec.resize)
	}
	return s.drain(proc, spec)
}

// commandOf builds the executable and arguments. With env, env(1) sets the
// variables and then looks the command up, so tools installed under the home
// directory are found. Nothing is parsed by a shell.
func commandOf(argv, env []string) (string, []string) {
	if len(env) == 0 {
		return argv[0], argv[1:]
	}
	return "env", append(slices.Clone(env), argv...)
}

func termSize(size sandbox.TermSize) (rows, cols uint16) {
	if size.Rows == 0 || size.Cols == 0 {
		return defaultRows, defaultCols
	}
	return size.Rows, size.Cols
}

// pumpInput copies in to the command. It is not awaited: it may be blocked
// reading a terminal when the command ends.
func pumpInput(ctx context.Context, proc Process, in io.Reader, tty bool) {
	if in == nil {
		return
	}
	buf := make([]byte, inputChunk)
	for {
		n, readErr := in.Read(buf)
		if n > 0 {
			if err := proc.SendInput(ctx, slices.Clone(buf[:n])); err != nil {
				return
			}
		}
		if readErr != nil {
			if !tty {
				_ = proc.CloseStdin(ctx)
			}
			return
		}
	}
}

func watchResize(ctx context.Context, proc Process, resize <-chan sandbox.TermSize) {
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
				_ = proc.Resize(ctx, size.Rows, size.Cols)
			}
		}
	}
}

// drain copies output until the command ends. A failed write does not stop
// the loop: the command must still be seen to finish.
func (s *session) drain(proc Process, spec runSpec) error {
	var copyErr error
	for {
		ev, err := proc.Next()
		if err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				return errors.New("lost the connection to the sandbox; run the command again")
			}
			return s.err(err)
		}
		if ev.Kind == EventEnd {
			return s.exitOf(ev, copyErr)
		}
		w := spec.stdout
		if ev.Kind == EventStderr {
			w = spec.stderr
		}
		if w == nil {
			continue
		}
		if _, werr := w.Write(ev.Data); werr != nil && copyErr == nil {
			copyErr = werr
		}
	}
}

func (s *session) exitOf(ev Event, copyErr error) error {
	switch {
	case ev.ExitCode != 0:
		// A command stopped by a signal reports a negative code.
		return &sandbox.ExitError{Code: max(ev.ExitCode, 1)}
	case ev.Err != "":
		return s.err(fmt.Errorf("command failed: %s", ev.Err))
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
		argv: argv, dir: dir, env: append(e2bLayout.Env(), env...),
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
