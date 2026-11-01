package daytona

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	sdk "github.com/daytona/clients/sdk-go/pkg/daytona"
	"github.com/daytona/clients/sdk-go/pkg/options"
	"github.com/daytona/clients/sdk-go/pkg/types"

	"github.com/perylemke/kaxa/internal/sandbox"
)

const (
	// logPoll is how often a running command is asked for its output and status.
	logPoll      = 300 * time.Millisecond
	logPollFirst = 25 * time.Millisecond
	// maxStagedInput caps the standard input of a command that is not a
	// terminal. It is uploaded as a file before the command starts.
	maxStagedInput = 64 << 20
	inputChunk     = 32 << 10
	termName       = "xterm-256color"
)

// toolboxClient runs commands in a sandbox and moves files into it.
type toolboxClient interface {
	// Run runs one command to its end and returns its exit code. Output goes to
	// the writers of spec. With spec.tty the command runs in a terminal.
	Run(ctx context.Context, id string, spec runSpec) (int, error)
	// Spawn starts a command that keeps running after kaxa leaves.
	Spawn(ctx context.Context, id string, spec runSpec) (process, error)
	// Upload writes r to path in the sandbox.
	Upload(ctx context.Context, id, path string, r io.Reader) error
}

// process is a command started by Spawn.
type process interface {
	// Wait waits up to window for the command to end and reports its exit code.
	Wait(ctx context.Context, window time.Duration) (code int, done bool, err error)
	// Output returns what the command printed, within a size limit.
	Output(ctx context.Context) string
}

// sdkToolbox is the toolboxClient over the Daytona Go SDK. Commands run in
// sessions of the sandbox daemon, and terminals use its PTY.
type sdkToolbox struct {
	c *sdkClient
}

func (t *sdkToolbox) sandbox(ctx context.Context, id string) (*sdk.Sandbox, error) {
	return t.c.handle(ctx, id)
}

// quote quotes s for a POSIX shell. The sandbox daemon takes one command
// string, so every word is quoted and nothing the user typed is parsed.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// commandLine builds the command string for a sandbox session: the directory,
// then env(1) to set the variables and look the command up in the PATH they set.
func commandLine(spec runSpec, prefix string) string {
	var b strings.Builder
	if spec.dir != "" {
		b.WriteString("cd " + quote(spec.dir) + " && ")
	}
	b.WriteString(prefix)
	if len(spec.env) > 0 {
		b.WriteString("env")
		for _, e := range spec.env {
			b.WriteString(" " + quote(e))
		}
		b.WriteString(" ")
	}
	words := make([]string, len(spec.argv))
	for i, w := range spec.argv {
		words[i] = quote(w)
	}
	b.WriteString(strings.Join(words, " "))
	return b.String()
}

func randomID(prefix string) (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("daytona: random id: %w", err)
	}
	return prefix + hex.EncodeToString(b), nil
}

func (t *sdkToolbox) Upload(ctx context.Context, id, path string, r io.Reader) error {
	sb, err := t.sandbox(ctx, id)
	if err != nil {
		return err
	}
	return convertErr(sb.FileSystem.UploadFileStream(ctx, r, path))
}

func (t *sdkToolbox) Run(ctx context.Context, id string, spec runSpec) (int, error) {
	sb, err := t.sandbox(ctx, id)
	if err != nil {
		return 0, err
	}
	if spec.tty {
		return t.runTerminal(ctx, sb, spec)
	}
	return t.runCommand(ctx, sb, spec)
}

func (t *sdkToolbox) runCommand(ctx context.Context, sb *sdk.Sandbox, spec runSpec) (int, error) {
	staged := ""
	if spec.stdin != nil {
		path, err := t.stage(ctx, sb, spec.stdin)
		if err != nil {
			return 0, err
		}
		defer t.remove(ctx, sb, path)
		staged = path
	}
	session, err := randomID("kaxa-")
	if err != nil {
		return 0, err
	}
	if err := sb.Process.CreateSession(ctx, session); err != nil {
		return 0, convertErr(err)
	}
	// The session is deleted once, either while the last output is read or when
	// the command fails before that.
	deleteSession := sync.OnceFunc(func() {
		bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
		defer cancel()
		_ = sb.Process.DeleteSession(bg, session)
	})
	defer deleteSession()
	line := commandLine(spec, "")
	if staged != "" {
		line += " < " + quote(staged)
	}
	started, err := sb.Process.ExecuteSessionCommand(ctx, session, line, true, false)
	if err != nil {
		return 0, convertErr(err)
	}
	cmdID, _ := started["id"].(string)
	return t.follow(ctx, sb, session, cmdID, spec, deleteSession)
}

// follow copies the output of a command to the writers of spec until the
// command ends, and returns its exit code. The daemon keeps the whole log, so
// each poll writes only what is new. Every call costs a round trip, so the
// first polls come fast, which is when a short command ends, and when the
// command is done the last read of the log runs while the session is deleted.
func (t *sdkToolbox) follow(ctx context.Context, sb *sdk.Sandbox, session, cmdID string, spec runSpec, cleanup func()) (int, error) {
	var seenOut, seenErr int
	var copyErr error
	drain := func() error {
		logs, err := sb.Process.GetSessionCommandLogs(ctx, session, cmdID)
		if err != nil {
			return convertErr(err)
		}
		seenOut = emit(spec.stdout, logs.GetStdout(), seenOut, &copyErr)
		seenErr = emit(spec.stderr, logs.GetStderr(), seenErr, &copyErr)
		return nil
	}
	for poll := 0; ; poll++ {
		code, done, err := commandStatus(ctx, sb, session, cmdID)
		if err != nil {
			return 0, err
		}
		if done {
			cleaned := make(chan struct{})
			go func() {
				defer close(cleaned)
				cleanup()
			}()
			err := drain()
			<-cleaned
			if err != nil {
				return 0, err
			}
			return code, copyErr
		}
		if err := drain(); err != nil {
			return 0, err
		}
		if err := sleep(ctx, pollDelay(poll)); err != nil {
			return 0, err
		}
	}
}

// pollDelay is the wait before poll number n+1: short at first, then up to logPoll.
func pollDelay(n int) time.Duration {
	return min(logPollFirst<<min(n, 8), logPoll)
}

// emit writes the part of text after the first seen bytes and returns the new count.
func emit(w io.Writer, text string, seen int, copyErr *error) int {
	if len(text) <= seen {
		return seen
	}
	if w != nil {
		if _, err := io.WriteString(w, text[seen:]); err != nil && *copyErr == nil {
			*copyErr = err
		}
	}
	return len(text)
}

// commandStatus reports whether a session command ended and its exit code.
func commandStatus(ctx context.Context, sb *sdk.Sandbox, session, cmdID string) (code int, done bool, err error) {
	st, err := sb.Process.GetSessionCommand(ctx, session, cmdID)
	if err != nil {
		return 0, false, convertErr(err)
	}
	raw, ok := st["exitCode"]
	if !ok {
		return 0, false, nil
	}
	switch v := raw.(type) {
	case int32:
		return int(v), true, nil
	case int:
		return v, true, nil
	case int64:
		return int(v), true, nil
	case float64:
		return int(v), true, nil
	}
	return 0, true, fmt.Errorf("daytona: unexpected exit code type %T", raw)
}

// stage uploads the standard input of a command to a file in the sandbox.
func (t *sdkToolbox) stage(ctx context.Context, sb *sdk.Sandbox, in io.Reader) (string, error) {
	buffered := bufio.NewReader(in)
	if _, err := buffered.Peek(1); err != nil && errors.Is(err, io.EOF) {
		// No input at all, which is what a closed stdin looks like: skip the
		// upload and the cleanup, two round trips.
		return "", nil
	}
	path, err := randomID("/tmp/kaxa-in-")
	if err != nil {
		return "", err
	}
	limited := &io.LimitedReader{R: buffered, N: maxStagedInput + 1}
	if err := sb.FileSystem.UploadFileStream(ctx, limited, path); err != nil {
		return "", convertErr(err)
	}
	if limited.N <= 0 {
		t.remove(ctx, sb, path)
		return "", fmt.Errorf("daytona: standard input is larger than %d MiB", maxStagedInput>>20)
	}
	return path, nil
}

func (t *sdkToolbox) remove(ctx context.Context, sb *sdk.Sandbox, path string) {
	bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	_, _ = sb.Process.ExecuteCommand(bg, "rm -f "+quote(path))
}

func (t *sdkToolbox) runTerminal(ctx context.Context, sb *sdk.Sandbox, spec runSpec) (int, error) {
	rows, cols := termSize(spec.size)
	id, err := randomID("kaxa-pty-")
	if err != nil {
		return 0, err
	}
	pty, err := sb.Process.CreatePty(ctx, id,
		options.WithCreatePtySize(types.PtySize{Rows: int(rows), Cols: int(cols)}),
		options.WithCreatePtyEnv(map[string]string{"TERM": termName}),
	)
	if err != nil {
		return 0, convertErr(err)
	}
	defer func() { _ = pty.Disconnect() }()
	if err := pty.WaitForConnection(ctx); err != nil {
		return 0, convertErr(err)
	}
	// The terminal is the login shell of the sandbox; exec replaces it with the
	// command, so the terminal ends when the command does.
	if err := pty.SendInput([]byte(commandLine(spec, "exec ") + "\n")); err != nil {
		return 0, convertErr(err)
	}
	go pumpTerminal(pty, spec.stdin)
	go watchResize(ctx, pty, spec.resize)

	var copyErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		for data := range pty.DataChan() {
			if spec.stdout == nil {
				continue
			}
			if _, werr := spec.stdout.Write(data); werr != nil && copyErr == nil {
				copyErr = werr
			}
		}
	}()
	res, err := pty.Wait(ctx)
	if err != nil {
		return 0, convertErr(err)
	}
	select {
	case <-done:
	case <-time.After(cleanupTimeout):
	}
	if res.ExitCode == nil {
		return 0, errors.New("daytona: terminal ended without an exit code")
	}
	return *res.ExitCode, copyErr
}

// pumpTerminal copies in to the terminal. It is not awaited: it may be blocked
// reading when the command ends. A terminal has no end of input.
func pumpTerminal(pty *sdk.PtyHandle, in io.Reader) {
	if in == nil {
		return
	}
	buf := make([]byte, inputChunk)
	for {
		n, readErr := in.Read(buf)
		if n > 0 {
			if err := pty.SendInput(slices.Clone(buf[:n])); err != nil {
				return
			}
		}
		if readErr != nil {
			return
		}
	}
}

func watchResize(ctx context.Context, pty *sdk.PtyHandle, resize <-chan sandbox.TermSize) {
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
				_, _ = pty.Resize(ctx, int(size.Cols), int(size.Rows))
			}
		}
	}
}

func (t *sdkToolbox) Spawn(ctx context.Context, id string, spec runSpec) (process, error) {
	sb, err := t.sandbox(ctx, id)
	if err != nil {
		return nil, err
	}
	session, err := randomID("kaxa-svc-")
	if err != nil {
		return nil, err
	}
	if err := sb.Process.CreateSession(ctx, session); err != nil {
		return nil, convertErr(err)
	}
	started, err := sb.Process.ExecuteSessionCommand(ctx, session, commandLine(spec, ""), true, false)
	if err != nil {
		return nil, convertErr(err)
	}
	cmdID, _ := started["id"].(string)
	return &sessionProcess{sb: sb, session: session, cmdID: cmdID}, nil
}

// sessionProcess is a command running in a session that outlives kaxa.
type sessionProcess struct {
	sb      *sdk.Sandbox
	session string
	cmdID   string
}

func (p *sessionProcess) Wait(ctx context.Context, window time.Duration) (int, bool, error) {
	deadline := time.Now().Add(window)
	for {
		code, done, err := commandStatus(ctx, p.sb, p.session, p.cmdID)
		if err != nil || done {
			return code, done, err
		}
		left := time.Until(deadline)
		if left <= 0 {
			return 0, false, nil
		}
		if err := sleep(ctx, min(logPoll, left)); err != nil {
			return 0, false, err
		}
	}
}

func (p *sessionProcess) Output(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	logs, err := p.sb.Process.GetSessionCommandLogs(ctx, p.session, p.cmdID)
	if err != nil {
		return ""
	}
	out := logs.GetOutput()
	if len(out) > failureOutput {
		out = out[:failureOutput]
	}
	return out
}
