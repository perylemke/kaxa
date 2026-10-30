package docker

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	apiDir  = workspaceDir + "/api"
	uvicorn = "uvicorn"
)

var apiArgv = []string{uvicorn, "app.main:app", "--host", "0.0.0.0"}

// isProbe reports whether spec is the readiness check, which runs bash.
func isProbe(spec ProcessSpec) bool {
	return len(spec.Argv) > 0 && spec.Argv[0] == "bash"
}

// publishProvider is a provider with a running sandbox that was synced.
func publishProvider(t *testing.T) (*Provider, *fakeAPI, *fakeStarter) {
	t.Helper()
	p, api, proc := newTestProvider(t)
	createBox(t, p, labelKaxa, sandbox.SyncLabel)
	p.SetReadiness(time.Second, time.Millisecond)
	return p, api, proc
}

// probeAnswers makes the readiness probe fail ready times and then succeed;
// a negative ready makes it fail for ever. It reports the ports published
// when a probe ran, and the number of probes.
func probeAnswers(api *fakeAPI, proc *fakeStarter, ready int) (probes, early *atomic.Int32) {
	probes, early = new(atomic.Int32), new(atomic.Int32)
	proc.script = func(spec ProcessSpec, _ string) []Event {
		if !isProbe(spec) {
			return nil
		}
		n := probes.Add(1)
		if api.seen("publish") > 0 {
			early.Add(1)
		}
		if ready >= 0 && int(n) > ready {
			return nil
		}
		return []Event{exited(1)}
	}
	return probes, early
}

func TestPublishAPIStartsServiceAndPublishesThePort(t *testing.T) {
	t.Parallel()
	p, api, proc := publishProvider(t)
	probes, early := probeAnswers(api, proc, 2)

	err := p.PublishAPI(context.Background(), testSandbox, apiDir, 8000, apiArgv, []string{"MODE=prod"})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if probes.Load() != 3 {
		t.Fatalf("probes = %d, want 3", probes.Load())
	}
	if early.Load() != 0 {
		t.Fatal("the port was published before the service listened")
	}
	if !slices.Equal(api.published, []int{8000}) {
		t.Fatalf("published = %v", api.published)
	}
	checkServiceSpec(t, proc)
	probe := proc.specs[1]
	if !isProbe(probe) || probe.Argv[len(probe.Argv)-1] != "8000" {
		t.Fatalf("probe = %+v", probe)
	}
	if proc.tokens[0] != testToken {
		t.Fatalf("service credential = %q", proc.tokens[0])
	}
}

// checkServiceSpec checks the first process of PublishAPI: the service.
func checkServiceSpec(t *testing.T, proc *fakeStarter) {
	t.Helper()
	svc := proc.specs[0]
	if svc.Argv[0] != cmdEnv || svc.Dir != apiDir || svc.User != agentUser || svc.TTY {
		t.Fatalf("service spec = %+v", svc)
	}
	if !slices.Contains(svc.Argv, "MODE=prod") || !slices.Contains(svc.Argv, "HOME="+agentHome) {
		t.Fatalf("service argv = %v", svc.Argv)
	}
	if !slices.Equal(svc.Argv[len(svc.Argv)-len(apiArgv):], apiArgv) {
		t.Fatalf("service argv = %v, want the command last", svc.Argv)
	}
}

func TestPublishAPIServiceExits(t *testing.T) {
	t.Parallel()
	p, api, proc := publishProvider(t)
	probeAnswers(api, proc, -1)
	script := proc.script
	proc.script = func(spec ProcessSpec, stdin string) []Event {
		if isProbe(spec) {
			return script(spec, stdin)
		}
		return []Event{errOutput("ModuleNotFoundError: no module named app\n"), exited(1)}
	}
	proc.immediate = func(spec ProcessSpec) bool { return !isProbe(spec) }

	err := p.PublishAPI(context.Background(), testSandbox, apiDir, 8000, apiArgv, nil)
	ue, ok := usererr.As(err)
	if !ok || !strings.Contains(ue.Message, "exit status 1") || !strings.Contains(ue.Message, "ModuleNotFoundError") {
		t.Fatalf("err = %v", err)
	}
	if len(api.published) != 0 {
		t.Fatalf("published %v for a service that stopped", api.published)
	}
}

func TestPublishAPITimeout(t *testing.T) {
	t.Parallel()
	p, api, proc := publishProvider(t)
	p.SetReadiness(20*time.Millisecond, time.Millisecond)
	probes, _ := probeAnswers(api, proc, -1)

	err := p.PublishAPI(context.Background(), testSandbox, apiDir, 8000, apiArgv, nil)
	ue, ok := usererr.As(err)
	if !ok || !strings.Contains(ue.Message, "did not start listening") || probes.Load() == 0 {
		t.Fatalf("err = %v probes = %d", err, probes.Load())
	}
	if len(api.published) != 0 {
		t.Fatalf("published %v for a service that never listened", api.published)
	}
}

func TestPublishAPIStopsWhenContextIsCanceled(t *testing.T) {
	t.Parallel()
	p, api, proc := publishProvider(t)
	probeAnswers(api, proc, -1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := p.PublishAPI(ctx, testSandbox, apiDir, 8000, apiArgv, nil); err == nil {
		t.Fatal("canceled publish succeeded")
	}
	if len(api.published) != 0 {
		t.Fatalf("published %v after cancel", api.published)
	}
}

func TestPublishAPIValidatesInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		dir  string
		port int
		argv []string
		env  []string
	}{
		{name: "dir outside the workspace", dir: "/etc", port: 8000, argv: apiArgv},
		{name: "dir with traversal", dir: workspaceDir + "/../x", port: 8000, argv: apiArgv},
		{name: "nested dir", dir: apiDir + "/sub", port: 8000, argv: apiArgv},
		{name: "workspace itself", dir: workspaceDir, port: 8000, argv: apiArgv},
		{name: "port zero", dir: apiDir, port: 0, argv: apiArgv},
		{name: "port too high", dir: apiDir, port: 70000, argv: apiArgv},
		{name: "no command", dir: apiDir, port: 8000},
		{name: "flag as binary", dir: apiDir, port: 8000, argv: []string{"--host", "x"}},
		{name: "empty argument", dir: apiDir, port: 8000, argv: []string{uvicorn, ""}},
		{name: "env without equals", dir: apiDir, port: 8000, argv: apiArgv, env: []string{"NOPE"}},
		{name: "env bad key", dir: apiDir, port: 8000, argv: apiArgv, env: []string{"1A=b"}},
		{name: "env line break", dir: apiDir, port: 8000, argv: apiArgv, env: []string{"A=b\nC=d"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, api, proc := publishProvider(t)
			calls := len(api.calls)
			if err := p.PublishAPI(context.Background(), testSandbox, tt.dir, tt.port, tt.argv, tt.env); err == nil {
				t.Fatal("invalid input accepted")
			}
			if len(proc.specs) != 0 || len(api.published) != 0 || len(api.calls) != calls {
				t.Fatalf("touched the sandbox: %d processes, calls %v", len(proc.specs), api.calls[calls:])
			}
		})
	}
}

func TestPublishAPIAcceptsTheHomeDirectory(t *testing.T) {
	t.Parallel()
	p, api, proc := publishProvider(t)
	probeAnswers(api, proc, 0)
	if err := p.PublishAPI(context.Background(), testSandbox, agentHome, 8000, apiArgv, nil); err != nil {
		t.Fatalf("publish: %v", err)
	}
}

func TestPublishAPIEnvErrorHidesValue(t *testing.T) {
	t.Parallel()
	p, _, _ := publishProvider(t)
	err := p.PublishAPI(context.Background(), testSandbox, apiDir, 8000, apiArgv, []string{"TOKEN"})
	if err == nil || strings.Contains(err.Error(), "TOKEN=") {
		t.Fatalf("err = %v", err)
	}
	err = p.PublishAPI(context.Background(), testSandbox, apiDir, 8000, apiArgv, []string{"SECRET=hunter2\n"})
	if err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("err leaks the value: %v", err)
	}
}

func TestPublishAPIPortErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		err      error
		wantMsg  string
		wantHint string
	}{
		{
			name:     "permission denied",
			err:      &APIError{Status: 403, Code: codePermissionDenied},
			wantMsg:  "denied publishing",
			wantHint: "sandboxesPorts",
		},
		{
			name:     "quota",
			err:      &APIError{Status: 429, Code: codeExhausted, Message: "limit"},
			wantMsg:  "limit reached",
			wantHint: "destroy sandboxes",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, api, proc := publishProvider(t)
			probeAnswers(api, proc, 0)
			api.failOn["publish"] = tt.err
			err := p.PublishAPI(context.Background(), testSandbox, apiDir, 8000, apiArgv, nil)
			ue, ok := usererr.As(err)
			if !ok || !strings.Contains(ue.Message, tt.wantMsg) || !strings.Contains(ue.Hint, tt.wantHint) {
				t.Fatalf("err = %v", err)
			}
			if !errors.Is(err, tt.err) {
				t.Fatalf("err = %v does not wrap the API error", err)
			}
		})
	}
}

func TestPublishAPIUnknownSandbox(t *testing.T) {
	t.Parallel()
	p, _, _ := publishProvider(t)
	err := p.PublishAPI(context.Background(), "nope", apiDir, 8000, apiArgv, nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestPublishAPIWithoutCredentials(t *testing.T) {
	t.Parallel()
	err := New().PublishAPI(context.Background(), testSandbox, apiDir, 8000, apiArgv, nil)
	if !errors.Is(err, sandbox.ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}
