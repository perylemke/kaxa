package blaxel

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
	failureOutput        = 2 << 10
)

// reservedPorts are taken by the Blaxel sandbox itself.
var reservedPorts = map[int]struct{}{80: {}, 443: {}, 8080: {}}

// SetReadiness sets how long PublishAPI waits for the service to listen.
// Tests use it to avoid long waits. Blaxel bounds one port wait near 60 seconds;
// the deadline here is the caller's context around that wait.
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

// PublishAPI starts an HTTP service in workDir and exposes port on a public
// preview named kaxa-api. The sandbox must have been created with that port.
// Anyone with the preview URL can call the service.
func (p *Provider) PublishAPI(ctx context.Context, id sandbox.ID, workDir string, port int, argv, env []string) error {
	if err := validateName(string(id)); err != nil {
		return err
	}
	if err := validateAPIWorkDir(workDir); err != nil {
		return err
	}
	if err := validateAPIPort(port); err != nil {
		return err
	}
	if err := validateAPIArgv(argv); err != nil {
		return err
	}
	if err := validateAPIEnv(env); err != nil {
		return err
	}
	rec, secrets, err := p.open(ctx, id)
	if err != nil {
		return err
	}
	if rec.APIPort != port {
		return apiPortMismatch(rec.APIPort, port)
	}
	if err := p.startService(ctx, rec.Name, workDir, port, argv, env, secrets); err != nil {
		return err
	}
	c, _, _, err := p.clients()
	if err != nil {
		return err
	}
	if _, err := c.CreatePreview(ctx, rec.Name, port); err != nil {
		return userErr(err, rec.Name, secrets...)
	}
	return nil
}

func (p *Provider) startService(ctx context.Context, name, workDir string, port int, argv, env, secrets []string) error {
	_, rt, _, err := p.clients()
	if err != nil {
		return err
	}
	timeout, _ := p.readiness()
	readyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	spec := runSpec{
		argv: argv, dir: workDir, port: port,
		env: append(blaxelLayout.Env(), env...),
	}
	if err := rt.Start(readyCtx, name, spec); err != nil {
		return userErr(err, name, secrets...)
	}
	return nil
}

func validateAPIPort(port int) error {
	if _, reserved := reservedPorts[port]; reserved {
		return usererr.New(
			fmt.Sprintf("port %d is reserved on Blaxel", port),
			"pick another --api-port; 80, 443, and 8080 belong to the sandbox",
		)
	}
	if port < 1 || port > 65535 {
		return usererr.New(fmt.Sprintf("invalid api port %d", port), "use a TCP port from 1 to 65535")
	}
	return nil
}

func apiPortMismatch(have, want int) error {
	if have == 0 {
		return usererr.New(
			"sandbox has no published port",
			"create it again with --api so Blaxel opens the port",
		)
	}
	return usererr.New(
		fmt.Sprintf("sandbox publishes port %d, not %d", have, want),
		"use the port from create --api-port",
	)
}

func validateAPIWorkDir(dir string) error {
	if dir == blaxelHome {
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

func apiServiceFailed(code int, output string) error {
	msg := "api service stopped"
	if code > 0 {
		msg += " with exit status " + strconv.Itoa(code)
	}
	if detail := strings.TrimSpace(output); detail != "" {
		msg += ": " + detail
	}
	return usererr.New(msg, "check the command and the port")
}
