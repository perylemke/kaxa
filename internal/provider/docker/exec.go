package docker

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
	inputChunk = 32 << 10
	termName   = "xterm-256color"
)

// session is a running sandbox with an endpoint credential, ready to run
// commands. It holds one credential for every command of one operation.
type session struct {
	api     apiClient
	proc    processStarter
	rec     Record
	cred    Credential
	secrets []string
}

// open finds the sandbox, starts it if Docker stopped it, extends its time to
// live and asks for an endpoint credential.
func (p *Provider) open(ctx context.Context, id sandbox.ID) (*session, error) {
	api, rec, err := p.find(ctx, id)
	if err != nil {
		return nil, err
	}
	_, proc := p.clients()
	if proc == nil {
		return nil, missingCredentialsErr()
	}
	// A fresh read gives the current status, etag and endpoint.
	rec, err = api.Get(ctx, rec.Resource)
	if err != nil {
		return nil, p.fail(api, err, string(id))
	}
	if rec, err = p.ensureRunning(ctx, api, rec); err != nil {
		return nil, p.fail(api, err, string(id))
	}
	if err := api.Renew(ctx, rec, sandboxTTL); err != nil {
		return nil, p.fail(api, err, string(id))
	}
	cred, err := api.Credential(ctx, rec)
	if err != nil {
		return nil, p.fail(api, err, string(id))
	}
	return &session{
		api: api, proc: proc, rec: rec, cred: cred,
		secrets: append(api.secrets(), cred.Token),
	}, nil
}

// ensureRunning starts a stopped sandbox and waits for one that is starting.
func (p *Provider) ensureRunning(ctx context.Context, api apiClient, rec Record) (Record, error) {
	switch rec.Status {
	case statusRunning:
		return rec, nil
	case statusStopped:
		if err := api.Start(ctx, rec); err != nil {
			return Record{}, err
		}
	}
	return p.waitRunning(ctx, api, rec)
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
// A sandbox created with --sync runs the command in /home/agent/workspace with
// the installed toolchains on PATH; any other runs it where the image starts.
// Docker stops an idle sandbox; Exec starts it again first.
func (p *Provider) Exec(ctx context.Context, id sandbox.ID, opts sandbox.ExecOptions) error {
	if len(opts.Command) == 0 {
		return errors.New("docker exec: empty command")
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
		spec.dir, spec.env = workspaceDir, cloudLayout.Env()
	}
	return s.run(ctx, spec)
}

func (s *session) run(ctx context.Context, spec runSpec) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ps := ProcessSpec{Argv: commandOf(spec.argv, spec.env), Dir: spec.dir, User: agentUser, TTY: spec.tty}
	if spec.tty {
		ps.Rows, ps.Cols = spec.size.Rows, spec.size.Cols
		ps.Env = map[string]string{"TERM": termName}
	}
	proc, err := s.proc.Start(ctx, s.rec.Endpoint, s.cred, ps)
	if err != nil {
		return s.err(err)
	}
	defer func() { _ = proc.Close() }()

	// A canceled context closes the stream, which unblocks Next.
	go func() {
		<-ctx.Done()
		_ = proc.Close()
	}()
	go pumpInput(proc, spec.stdin, spec.tty)
	if spec.tty {
		go watchResize(ctx, proc, spec.resize)
	}
	return s.drain(ctx, proc, spec)
}

// commandOf builds the command line. With env, env(1) sets the variables and
// then looks the command up, so tools installed under the home directory are
// found. Nothing is parsed by a shell.
func commandOf(argv, env []string) []string {
	if len(env) == 0 {
		return slices.Clone(argv)
	}
	return slices.Concat([]string{"env"}, env, argv)
}

// pumpInput copies in to the command. It is not awaited: it may be blocked
// reading a terminal when the command ends. A command with no input gets end
// of file at once, so one that reads standard input does not wait for it.
func pumpInput(proc Process, in io.Reader, tty bool) {
	if in != nil {
		buf := make([]byte, inputChunk)
		for {
			n, readErr := in.Read(buf)
			if n > 0 {
				if err := proc.WriteStdin(buf[:n]); err != nil {
					return
				}
			}
			if readErr != nil {
				break
			}
		}
	}
	if !tty {
		_ = proc.CloseStdin()
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
				_ = proc.Resize(size.Rows, size.Cols)
			}
		}
	}
}

// drain copies output until the command ends. A failed write does not stop
// the loop: the command must still be seen to finish.
func (s *session) drain(ctx context.Context, proc Process, spec runSpec) error {
	var copyErr error
	for {
		ev, err := proc.Next()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return s.err(err)
		}
		if ev.Exited {
			if ev.ExitCode != 0 {
				// A command stopped by a signal reports a negative code.
				return &sandbox.ExitError{Code: max(ev.ExitCode, 1)}
			}
			return s.err(copyErr)
		}
		w := spec.stdout
		if ev.Stderr {
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
		argv: argv, dir: dir, env: env,
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
