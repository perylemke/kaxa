package docker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	defaultReadyTimeout  = 90 * time.Second
	defaultReadyInterval = time.Second
	// probeScript opens a TCP connection to the port in $1. The port is a
	// validated integer, and nothing the user typed reaches the script.
	probeScript = `exec 3<>"/dev/tcp/127.0.0.1/$1"`
	// failureOutput is how much of a failed service's output is shown.
	failureOutput = 2 << 10
)

// SetReadiness sets how long PublishAPI waits for the service to listen and
// how often it checks. Tests use it to avoid long waits.
func (p *Provider) SetReadiness(timeout, interval time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.readyTimeout, p.readyInterval = timeout, interval
}

func (p *Provider) readiness() (timeout, interval time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	timeout, interval = p.readyTimeout, p.readyInterval
	if timeout <= 0 {
		timeout = defaultReadyTimeout
	}
	if interval <= 0 {
		interval = defaultReadyInterval
	}
	return timeout, interval
}

// PublishAPI starts an HTTP service in workDir, waits until it listens on
// port, and asks Docker to publish that port. The URL appears in Inspect.
//
// The service keeps running in the sandbox, but it ends when Docker stops the
// sandbox, and kaxa does not start it again.
func (p *Provider) PublishAPI(ctx context.Context, id sandbox.ID, workDir string, port int, argv, env []string) error {
	if err := validateName(string(id)); err != nil {
		return err
	}
	if err := validateAPIWorkDir(workDir); err != nil {
		return err
	}
	if port < 1 || port > 65535 {
		return usererr.New(fmt.Sprintf("invalid api port %d", port), "use a TCP port from 1 to 65535")
	}
	if err := validateAPIArgv(argv); err != nil {
		return err
	}
	if err := validateAPIEnv(env); err != nil {
		return err
	}
	s, err := p.open(ctx, id)
	if err != nil {
		return err
	}
	if err := p.startService(ctx, s, workDir, port, argv, env); err != nil {
		return err
	}
	if _, err := s.api.PublishPort(ctx, s.rec, port); err != nil {
		return p.fail(s.api, publishErr(err), string(id))
	}
	return nil
}

// publishErr explains the refusals of the port API that the generic mapping
// would describe as a problem of the account.
func publishErr(err error) error {
	apiErr, ok := errors.AsType[*APIError](err)
	if ok && apiErr.is(codePermissionDenied, http.StatusForbidden) {
		return usererr.Wrap(apiErr, "Docker denied publishing the port",
			"the token needs permission to publish sandbox ports (sandboxesPorts); check the subscription and the scopes of the personal access token")
	}
	return err
}

func (p *Provider) startService(ctx context.Context, s *session, workDir string, port int, argv, env []string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	spec := ProcessSpec{Argv: commandOf(argv, slices.Concat(cloudLayout.Env(), env)), Dir: workDir, User: agentUser}
	proc, err := s.proc.Start(ctx, s.rec.Endpoint, s.cred, spec)
	if err != nil {
		return fmt.Errorf("start api service: %w", s.err(err))
	}
	// Closing the stream drops only our connection; the process is durable
	// and the service keeps running.
	defer func() { _ = proc.Close() }()

	out := sandbox.NewCappedBuffer(failureOutput)
	exited := make(chan Event, 1)
	go watchService(proc, out, exited)

	timeout, interval := p.readiness()
	return s.waitReady(ctx, port, timeout, interval, exited, out)
}

// watchService collects the output of the service and reports when it ends.
func watchService(proc Process, out *sandbox.CappedBuffer, exited chan<- Event) {
	for {
		ev, err := proc.Next()
		if err != nil {
			exited <- Event{Exited: true, ExitCode: -1, Data: []byte(err.Error())}
			return
		}
		if ev.Exited {
			exited <- ev
			return
		}
		_, _ = out.Write(ev.Data)
	}
}

func (s *session) waitReady(ctx context.Context, port int, timeout, interval time.Duration, exited <-chan Event, out *sandbox.CappedBuffer) error {
	deadline := time.Now().Add(timeout)
	for {
		select {
		case ev := <-exited:
			return apiServiceFailed(ev, out.String())
		default:
		}
		if s.listening(ctx, port) {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !time.Now().Before(deadline) {
			return usererr.New("api service did not start listening", "check the command and the port, and listen on 0.0.0.0")
		}
		if err := sleep(ctx, interval); err != nil {
			return err
		}
	}
}

func (s *session) listening(ctx context.Context, port int) bool {
	_, err := s.capture(ctx, "", nil, []string{"bash", "-c", probeScript, "kaxa", strconv.Itoa(port)}, nil)
	return err == nil
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// apiServiceFailed describes a service that ended. For a stream that broke,
// the exit event carries the reason in Data instead of an exit code.
func apiServiceFailed(ev Event, output string) error {
	msg := "api service stopped"
	if ev.ExitCode > 0 {
		msg += " with exit status " + strconv.Itoa(ev.ExitCode)
	}
	if detail := strings.TrimSpace(output); detail != "" {
		msg += ": " + detail
	} else if reason := strings.TrimSpace(string(ev.Data)); reason != "" {
		msg += ": " + reason
	}
	return usererr.New(msg, "check the command and the port")
}

func validateAPIWorkDir(dir string) error {
	if dir == agentHome {
		return nil
	}
	rest, ok := strings.CutPrefix(dir, workspaceDir+"/")
	if !ok || rest == "" || strings.Contains(rest, "/") {
		return usererr.New(fmt.Sprintf("invalid api directory %q", dir), "use the home directory or a workspace repository")
	}
	return validateRepoDir(rest)
}

func validateAPIArgv(argv []string) error {
	if len(argv) == 0 {
		return usererr.New("api command is required", "pass --api-cmd with comma-separated argv")
	}
	if strings.HasPrefix(argv[0], "-") {
		return usererr.New("api command must start with a binary", "put the binary first in --api-cmd")
	}
	if slices.Contains(argv, "") {
		return usererr.New("api command has an empty argument", "separate argv with commas and omit empty items")
	}
	return nil
}

// validateAPIEnv is a last check of the variables. The error names no value,
// which can be a secret.
func validateAPIEnv(env []string) error {
	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || !sandbox.ValidEnvKey(key) || strings.ContainsAny(entry, "\n\r\x00") {
			return usererr.New("invalid api env entry", "use KEY=VALUE without line breaks")
		}
	}
	return nil
}
