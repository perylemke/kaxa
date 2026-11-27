package vercel

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
	// shellBash and wrapperArg0 name the shell and the $0 of the wrapper scripts.
	shellBash   = "bash"
	wrapperArg0 = "kaxa"
	// stdinChunk is the most input sent per request. Vercel accepts 1 MB of
	// base64 per request, which is about 700 KB of data.
	stdinChunk = 256 << 10
	// pathScript puts the toolchains kaxa installs ahead of the PATH of the
	// sandbox and runs the rest of the arguments. The script is fixed text;
	// everything the user typed arrives as separate arguments.
	pathScript = `export PATH="$1:$PATH"; shift; exec "$@"`
)

// runSpec is one command of a session.
type runSpec struct {
	argv []string
	dir  string
	// env is KEY=VALUE entries added to the environment of the command.
	env    []string
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// Exec runs a command in a sandbox and returns when it exits. A non-zero exit
// code is returned as *sandbox.ExitError.
//
// A sandbox created with --sync runs the command in the workspace; any other
// runs it in the home directory. opts.Dir replaces that directory. Vercel
// resumes a stopped sandbox first. With TTY set, the command runs in an
// interactive terminal hosted by Vercel; see console.go.
func (p *Provider) Exec(ctx context.Context, id sandbox.ID, opts sandbox.ExecOptions) error {
	if len(opts.Command) == 0 {
		return errors.New("vercel exec: empty command")
	}
	if _, err := sandbox.ResolveExecDir(opts.Dir, ""); err != nil {
		return err
	}
	s, err := p.open(ctx, id)
	if err != nil {
		return err
	}
	fallback := vercelHome
	if s.synced() {
		fallback = workspaceDir
	}
	dir, err := sandbox.ResolveExecDir(opts.Dir, fallback)
	if err != nil {
		return err
	}
	if opts.TTY {
		return s.console(ctx, consoleSpecFor(opts, dir))
	}
	return s.run(ctx, runSpec{
		argv: opts.Command, dir: dir, stdin: opts.Stdin, stdout: opts.Stdout, stderr: opts.Stderr,
	})
}

// request builds the command. Without an explicit PATH, the toolchain
// directories are put in front of the PATH of the sandbox.
func (spec runSpec) request() CmdRequest {
	req := CmdRequest{Command: spec.argv[0], Args: spec.argv[1:], Cwd: spec.dir, Stdin: spec.stdin != nil}
	if len(spec.env) > 0 {
		req.Env = make(map[string]string, len(spec.env))
		for _, e := range spec.env {
			if k, v, ok := strings.Cut(e, "="); ok && k != "" {
				req.Env[k] = v
			}
		}
	}
	if _, hasPath := req.Env["PATH"]; !hasPath {
		req.Args = append([]string{"-c", pathScript, wrapperArg0, toolPath()}, spec.argv...)
		req.Command = shellBash
	}
	return req
}

func toolPath() string {
	return vercelLayout.Home + "/.local/bin:" + vercelLayout.Home + "/.local/go/bin:" + vercelLayout.Home + "/.cargo/bin"
}

func (s *session) run(ctx context.Context, spec runSpec) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	req := spec.request()
	stream, err := s.api.Run(ctx, s.rec.SessionID, req)
	if err != nil {
		return s.err(err)
	}
	defer func() { _ = stream.Close() }()
	return s.drain(ctx, stream, spec, req.Stdin)
}

// drain copies output until the command ends. A failed write does not stop the
// loop: the command must still be seen to finish.
func (s *session) drain(ctx context.Context, stream Stream, spec runSpec, stdin bool) error {
	var cmdID string
	var copyErr error
	for {
		ev, err := stream.Next()
		if err != nil {
			s.killOnCancel(ctx, cmdID)
			if errors.Is(err, io.ErrUnexpectedEOF) {
				return errors.New("lost the connection to the sandbox; run the command again")
			}
			return s.err(err)
		}
		switch ev.Kind {
		case EventStarted:
			cmdID = ev.CmdID
			if stdin {
				go s.pumpInput(ctx, cmdID, spec.stdin)
			}
		case EventStdout, EventStderr:
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
		case EventError:
			return s.err(fmt.Errorf("command failed: %s", ev.Err))
		case EventEnd:
			return s.exitOf(ev, copyErr)
		}
	}
}

func (s *session) exitOf(ev Event, copyErr error) error {
	if ev.ExitCode == nil {
		return s.err(errors.New("command ended without an exit code"))
	}
	if *ev.ExitCode != 0 {
		// A command stopped by a signal reports a negative code.
		return &sandbox.ExitError{Code: max(*ev.ExitCode, 1)}
	}
	return s.err(copyErr)
}

// killOnCancel stops a command whose caller gave up, so it does not keep
// running in the sandbox.
func (s *session) killOnCancel(ctx context.Context, cmdID string) {
	if cmdID == "" || ctx.Err() == nil {
		return
	}
	kctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	_ = s.api.Kill(kctx, s.rec.SessionID, cmdID)
}

// pumpInput copies in to the command and closes its stdin at the end. It is
// not awaited: it may be blocked reading when the command ends.
func (s *session) pumpInput(ctx context.Context, cmdID string, in io.Reader) {
	buf := make([]byte, stdinChunk)
	var offset int64
	for {
		n, readErr := in.Read(buf)
		if n > 0 {
			if err := s.api.WriteStdin(ctx, s.rec.SessionID, cmdID, slices.Clone(buf[:n]), offset, false); err != nil {
				return
			}
			offset += int64(n)
		}
		if readErr != nil {
			_ = s.api.WriteStdin(ctx, s.rec.SessionID, cmdID, nil, offset, true)
			return
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
	err := s.run(ctx, runSpec{argv: argv, dir: dir, env: env, stdin: stdin, stdout: stdout, stderr: stderr})
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
