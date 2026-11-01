package daytona

import (
	"context"
	"fmt"
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
	p.readyTimeout, p.readyEvery = timeout, interval
}

func (p *Provider) readiness() (timeout, interval time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	timeout, interval = p.readyTimeout, p.readyEvery
	if timeout <= 0 {
		timeout = defaultReadyTimeout
	}
	if interval <= 0 {
		interval = defaultReadyInterval
	}
	return timeout, interval
}

// PublishAPI starts an HTTP service in workDir, waits until it listens on
// port, and exposes the port as a public preview URL. The sandbox must have
// been created with an API on the same port, which is how kaxa knows the port.
//
// The service keeps running in the sandbox, but it does not survive a restart
// or a stop.
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
	if s.rec.APIPort != port {
		return apiPortMismatch(s.rec.APIPort, port)
	}
	if err := p.startService(ctx, s, workDir, port, argv, env); err != nil {
		return err
	}
	c, _, _, err := p.clients()
	if err != nil {
		return err
	}
	ep, err := c.SetPort(ctx, s.rec.ID, port, true)
	if err != nil {
		return fmt.Errorf("expose api port: %w", s.err(err))
	}
	if !ep.Public {
		return usererr.New("Daytona did not make the API port public", "check the port in the Daytona dashboard")
	}
	return nil
}

func apiPortMismatch(created, want int) error {
	if created == 0 {
		return usererr.New("this sandbox was not created with an API", "create a new sandbox with --api")
	}
	return usererr.New(
		fmt.Sprintf("this sandbox publishes port %d, not %d", created, want),
		"kaxa fixes the API port at creation; create a new sandbox with the port you want",
	)
}

func (p *Provider) startService(ctx context.Context, s *session, workDir string, port int, argv, env []string) error {
	id, err := s.tb.Start(ctx, s.conn, ProcessRequest{
		Kind: kindExec, Argv: commandOf(argv, append(daytonaLayout.Env(), env...)), Dir: workDir,
	})
	if err != nil {
		return fmt.Errorf("start api service: %w", s.err(err))
	}
	timeout, interval := p.readiness()
	deadline := time.Now().Add(timeout)
	for {
		res, err := s.tb.Wait(ctx, s.conn, id, interval)
		if err != nil {
			return s.err(err)
		}
		if res.Done() {
			return apiServiceFailed(res, s.serviceOutput(ctx, id))
		}
		if s.listening(ctx, port) {
			return nil
		}
		if !time.Now().Before(deadline) {
			return usererr.New("api service did not start listening", "check the command and the port, and listen on 0.0.0.0")
		}
	}
}

// serviceOutput reads what a failed service printed, within a size limit.
func (s *session) serviceOutput(ctx context.Context, id string) string {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	frames, err := s.tb.Logs(ctx, s.conn, id, "")
	if err != nil {
		return ""
	}
	out := sandbox.NewCappedBuffer(failureOutput)
	for _, f := range frames {
		_, _ = out.Write([]byte(f.Data))
	}
	return out.String()
}

func (s *session) listening(ctx context.Context, port int) bool {
	_, err := s.capture(ctx, "", nil, []string{"bash", "-c", probeScript, "kaxa", strconv.Itoa(port)}, nil)
	return err == nil
}

func apiServiceFailed(res WaitResult, output string) error {
	msg := "api service stopped"
	if res.HasCode && res.ExitCode > 0 {
		msg += " with exit status " + strconv.Itoa(res.ExitCode)
	}
	if detail := strings.TrimSpace(output); detail != "" {
		msg += ": " + detail
	}
	return usererr.New(msg, "check the command and the port")
}

func validateAPIWorkDir(dir string) error {
	if dir == daytonaHome {
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
