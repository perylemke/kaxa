package daytona

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
	defaultRows = 24
	defaultCols = 80
)

// session is a sandbox that is running and ready for commands.
type session struct {
	tb      toolboxClient
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
	return &session{tb: tb, rec: rec, secrets: secrets}, nil
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
	code, err := s.tb.Run(ctx, s.rec.ID, spec)
	if err != nil {
		return s.err(err)
	}
	if code != 0 {
		return &sandbox.ExitError{Code: code}
	}
	return nil
}

func termSize(size sandbox.TermSize) (rows, cols uint16) {
	if size.Rows == 0 || size.Cols == 0 {
		return defaultRows, defaultCols
	}
	return size.Rows, size.Cols
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
