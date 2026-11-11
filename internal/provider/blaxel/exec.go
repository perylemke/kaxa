package blaxel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

// Exec runs a command in a sandbox and returns when it exits. A non-zero exit
// code is returned as *sandbox.ExitError.
//
// With TTY, Exec attaches to the terminal Blaxel starts and ignores Command.
// A sandbox created with sync starts in /root/workspace; any other starts in
// /root. opts.Dir, when set, replaces that directory. The toolchain PATH is
// set for a command. An archived sandbox is left archived.
func (p *Provider) Exec(ctx context.Context, id sandbox.ID, opts sandbox.ExecOptions) error {
	if _, err := sandbox.ResolveExecDir(opts.Dir, ""); err != nil {
		return err
	}
	if opts.TTY {
		return p.console(ctx, id, opts)
	}
	if opts.Resize != nil {
		return usererr.New(
			"interactive shell is not available for blaxel",
			"run kaxa exec to run one command",
		)
	}
	if len(opts.Command) == 0 {
		return errors.New("blaxel exec: empty command")
	}
	rec, secrets, err := p.open(ctx, id)
	if err != nil {
		return err
	}
	_, rt, _, err := p.clients()
	if err != nil {
		return err
	}
	fallback := blaxelHome
	if p.synced(rec) {
		fallback = workspaceDir
	}
	dir, err := sandbox.ResolveExecDir(opts.Dir, fallback)
	if err != nil {
		return err
	}
	spec := runSpec{
		argv: opts.Command, stdin: pipedInput(opts.Stdin), stdout: opts.Stdout, stderr: opts.Stderr,
		env: blaxelLayout.Env(), dir: dir,
	}
	code, err := rt.Run(ctx, rec.Name, spec)
	if err != nil {
		return userErr(err, rec.Name, secrets...)
	}
	if code != 0 {
		return &sandbox.ExitError{Code: code}
	}
	return nil
}

// console attaches to the sandbox terminal. Blaxel starts the shell, so the
// command that kaxa console passes is not sent as keystrokes.
func (p *Provider) console(ctx context.Context, id sandbox.ID, opts sandbox.ExecOptions) error {
	rec, secrets, err := p.open(ctx, id)
	if err != nil {
		return err
	}
	_, rt, _, err := p.clients()
	if err != nil {
		return err
	}
	fallback := blaxelHome
	if p.synced(rec) {
		fallback = workspaceDir
	}
	dir, err := sandbox.ResolveExecDir(opts.Dir, fallback)
	if err != nil {
		return err
	}
	err = rt.Terminal(ctx, rec.Name, termSpec{
		dir: dir, stdin: opts.Stdin, stdout: opts.Stdout, stderr: opts.Stderr,
		rows: opts.Size.Rows, cols: opts.Size.Cols, resize: opts.Resize,
	})
	if err != nil {
		return userErr(err, rec.Name, secrets...)
	}
	return nil
}

// pipedInput returns r unless it is a character device, such as a terminal or
// /dev/null. Reading a terminal would make exec wait for Ctrl-D.
func pipedInput(r io.Reader) io.Reader {
	f, ok := r.(*os.File)
	if !ok {
		return r
	}
	info, err := f.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice != 0 {
		return nil
	}
	return r
}

// capture runs argv and returns its stdout. A non-zero exit is a plain error,
// not *sandbox.ExitError, so a failed copy still explains itself.
func (p *Provider) capture(ctx context.Context, id sandbox.ID, dir string, env, argv []string, stdin io.Reader) (string, error) {
	rec, secrets, err := p.open(ctx, id)
	if err != nil {
		return "", err
	}
	_, rt, _, err := p.clients()
	if err != nil {
		return "", err
	}
	stdout := sandbox.NewCappedBuffer(sandbox.MaxCapturedOutput)
	stderr := sandbox.NewCappedBuffer(sandbox.MaxCapturedOutput)
	code, err := rt.Run(ctx, rec.Name, runSpec{
		argv: argv, dir: dir, env: append(blaxelLayout.Env(), env...),
		stdin: stdin, stdout: stdout, stderr: stderr,
	})
	if err != nil {
		return "", userErr(err, rec.Name, secrets...)
	}
	if code == 0 {
		return stdout.String(), nil
	}
	msg := fmt.Sprintf("exit status %d", code)
	if detail := strings.TrimSpace(stderr.String()); detail != "" {
		msg += ": " + detail
	}
	return "", errors.New(msg)
}
