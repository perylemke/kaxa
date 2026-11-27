package vercel

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
	// probeScript opens a TCP connection to the port in $1. The port is a
	// validated integer, and nothing the user typed reaches the script.
	probeScript = `exec 3<>"/dev/tcp/127.0.0.1/$1"`
)

// PublishAPI starts an HTTP service in workDir and waits until it listens on
// port. The sandbox URL for that port is public. A sandbox created without
// that port gets it now, which restarts its session.
//
// The service keeps running while the session runs, but it does not survive a
// stop, a restart, or the end of the session timeout.
func (p *Provider) PublishAPI(ctx context.Context, id sandbox.ID, workDir string, port int, argv, env []string) error {
	if err := validateName(string(id)); err != nil {
		return err
	}
	if err := validateAPIWorkDir(workDir); err != nil {
		return err
	}
	if port < 1024 || port > 65535 || port == reservedPort {
		return usererr.New(
			fmt.Sprintf("invalid api port %d for vercel", port),
			fmt.Sprintf("use a port from 1024 to 65535 other than %d", reservedPort),
		)
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
	if !hasRoute(s.rec, port) {
		if s, err = p.addPort(ctx, s, port); err != nil {
			return err
		}
	}
	return p.startService(ctx, s, workDir, port, argv, env)
}

func hasRoute(rec Record, port int) bool {
	return slices.ContainsFunc(rec.Routes, func(r Route) bool { return r.Port == port })
}

// addPort exposes port and restarts the session so the route exists.
func (p *Provider) addPort(ctx context.Context, s *session, port int) (*session, error) {
	name := s.rec.Name
	patch := map[string]any{
		"ports": []int{port},
		"tags":  tagsOf(CreateParams{Labels: s.rec.Labels, APIPort: port}),
	}
	rec, err := p.cycle(ctx, s.api, name, func() error { return s.api.Update(ctx, name, patch) })
	if err != nil {
		return nil, s.err(err)
	}
	if !hasRoute(rec, port) {
		return nil, usererr.New(
			fmt.Sprintf("vercel did not open a route for port %d", port),
			"create a new sandbox with --api",
		)
	}
	return &session{api: s.api, rec: rec, secrets: s.secrets}, nil
}

func (p *Provider) startService(ctx context.Context, s *session, workDir string, port int, argv, env []string) error {
	req := runSpec{argv: argv, dir: workDir, env: env}.request()
	if _, err := s.api.Start(ctx, s.rec.SessionID, req); err != nil {
		return fmt.Errorf("start api service: %w", s.err(err))
	}
	timeout, interval := p.readiness()
	deadline := time.Now().Add(timeout)
	for {
		if s.listening(ctx, port) {
			return nil
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
	_, err := s.capture(ctx, "", nil, []string{shellBash, "-c", probeScript, wrapperArg0, strconv.Itoa(port)}, nil)
	return err == nil
}

func validateAPIWorkDir(dir string) error {
	if dir == vercelHome {
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
