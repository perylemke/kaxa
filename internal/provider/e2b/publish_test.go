package e2b_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/provider/e2b"
	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	apiDir = "/home/user/workspace/api"
	uvi    = "uvicorn"
)

var apiArgv = []string{uvi, "app.main:app", "--host", "0.0.0.0"}

// isProbe reports whether req is the readiness check, which runs bash through env.
func isProbe(req e2b.StartRequest) bool {
	return slices.Contains(req.Args, "bash") && slices.Contains(req.Args, probeName)
}

// serviceScript makes the service listen after ready probes, or never when
// ready is negative. The probe is the bash command that opens /dev/tcp.
func serviceScript(probes *int, ready int, service *fakeProc) func(e2b.StartRequest) *fakeProc {
	return func(req e2b.StartRequest) *fakeProc {
		if !isProbe(req) {
			return service
		}
		*probes++
		if ready >= 0 && *probes > ready {
			return exited(0)
		}
		return exited(1)
	}
}

func apiProvider(t *testing.T) (*e2b.Provider, *fakeAPI, *fakeEnvd) {
	t.Helper()
	p, api, envd := newProvider([]string{testKaxa, sandbox.SyncLabel}, 8000)
	p.SetReadiness(time.Second, time.Millisecond)
	return p, api, envd
}

func TestPublishAPIStartsServiceAndWaits(t *testing.T) {
	t.Parallel()

	p, _, envd := apiProvider(t)
	var probes int
	service := &fakeProc{block: make(chan struct{})}
	envd.script = serviceScript(&probes, 2, service)

	err := p.PublishAPI(context.Background(), testBox, apiDir, 8000, apiArgv, []string{"MODE=prod"})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if probes != 3 {
		t.Fatalf("probes = %d, want 3", probes)
	}
	svc := envd.starts[0]
	if svc.Cmd != "env" || svc.Cwd != apiDir || svc.User != testUser || svc.Stdin {
		t.Fatalf("service request = %+v", svc)
	}
	if !slices.Contains(svc.Args, "MODE=prod") || !slices.Contains(svc.Args, "HOME=/home/user") {
		t.Fatalf("service args = %v", svc.Args)
	}
	if !slices.Equal(svc.Args[len(svc.Args)-len(apiArgv):], apiArgv) {
		t.Fatalf("service args = %v, want argv last", svc.Args)
	}
	probe := envd.starts[1]
	if !isProbe(probe) || probe.Args[len(probe.Args)-1] != "8000" {
		t.Fatalf("probe = %+v", probe)
	}
	if !service.closed {
		t.Fatal("the stream to the service was left open")
	}
}

func TestPublishAPIServiceExits(t *testing.T) {
	t.Parallel()

	p, _, envd := apiProvider(t)
	var probes int
	crashed := output("", "ModuleNotFoundError: no module named app\n", 1)
	envd.script = serviceScript(&probes, -1, crashed)

	err := p.PublishAPI(context.Background(), testBox, apiDir, 8000, apiArgv, nil)
	ue, ok := usererr.As(err)
	if !ok || !strings.Contains(ue.Message, "exit status 1") || !strings.Contains(ue.Message, "ModuleNotFoundError") {
		t.Fatalf("err = %v", err)
	}
}

func TestPublishAPITimeout(t *testing.T) {
	t.Parallel()

	p, _, envd := apiProvider(t)
	p.SetReadiness(20*time.Millisecond, time.Millisecond)
	var probes int
	envd.script = serviceScript(&probes, -1, &fakeProc{block: make(chan struct{})})

	err := p.PublishAPI(context.Background(), testBox, apiDir, 8000, apiArgv, nil)
	ue, ok := usererr.As(err)
	if !ok || !strings.Contains(ue.Message, "did not start listening") || probes == 0 {
		t.Fatalf("err = %v probes = %d", err, probes)
	}
}

func TestPublishAPIStopsWhenContextIsCanceled(t *testing.T) {
	t.Parallel()

	p, _, envd := apiProvider(t)
	var probes int
	envd.script = serviceScript(&probes, -1, &fakeProc{block: make(chan struct{})})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := p.PublishAPI(ctx, testBox, apiDir, 8000, apiArgv, nil); err == nil {
		t.Fatal("canceled publish succeeded")
	}
}

func TestPublishAPIPortMustMatchTheSandbox(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		created int
		port    int
		wantErr string
	}{
		{name: "private sandbox", created: 0, port: 8000, wantErr: "not created with an API"},
		{name: "other port", created: 9000, port: 8000, wantErr: "publishes port 9000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, _, envd := newProvider(nil, tt.created)
			err := p.PublishAPI(context.Background(), testBox, "/home/user", tt.port, apiArgv, nil)
			ue, ok := usererr.As(err)
			if !ok || !strings.Contains(ue.Message, tt.wantErr) {
				t.Fatalf("err = %v", err)
			}
			if len(envd.starts) != 0 {
				t.Fatalf("started %v despite the error", envd.argvs())
			}
		})
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
		{name: "dir with traversal", dir: "/home/user/workspace/../x", port: 8000, argv: apiArgv},
		{name: "nested dir", dir: apiDir + "/sub", port: 8000, argv: apiArgv},
		{name: "port zero", dir: apiDir, port: 0, argv: apiArgv},
		{name: "port too high", dir: apiDir, port: 70000, argv: apiArgv},
		{name: "no command", dir: apiDir, port: 8000},
		{name: "flag as binary", dir: apiDir, port: 8000, argv: []string{"--host", "x"}},
		{name: "empty argument", dir: apiDir, port: 8000, argv: []string{uvi, ""}},
		{name: "env without equals", dir: apiDir, port: 8000, argv: apiArgv, env: []string{"NOPE"}},
		{name: "env bad key", dir: apiDir, port: 8000, argv: apiArgv, env: []string{"1A=b"}},
		{name: "env line break", dir: apiDir, port: 8000, argv: apiArgv, env: []string{"A=b\nC=d"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, api, envd := apiProvider(t)
			if err := p.PublishAPI(context.Background(), testBox, tt.dir, tt.port, tt.argv, tt.env); err == nil {
				t.Fatal("invalid input accepted")
			}
			if len(envd.starts) != 0 || len(api.connect) != 0 {
				t.Fatalf("touched the sandbox: starts = %d connects = %d", len(envd.starts), len(api.connect))
			}
		})
	}
}

func TestPublishAPIEnvErrorHidesValue(t *testing.T) {
	t.Parallel()

	p, _, _ := apiProvider(t)
	err := p.PublishAPI(context.Background(), testBox, apiDir, 8000, apiArgv, []string{"TOKEN"})
	if err == nil || strings.Contains(err.Error(), "TOKEN=") {
		t.Fatalf("err = %v", err)
	}
	err = p.PublishAPI(context.Background(), testBox, apiDir, 8000, apiArgv, []string{"SECRET=hunter2\n"})
	if err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("err leaks the value: %v", err)
	}
}

func TestRestartRebootsFromDisk(t *testing.T) {
	t.Parallel()

	p, api, _ := newProvider(nil, 0)
	id := api.boxes[testBox].ID

	if err := p.Restart(context.Background(), testBox); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if len(api.paused) != 1 || api.paused[0] != (pauseCall{id, false}) {
		t.Fatalf("paused = %+v, want a pause without memory", api.paused)
	}
	if len(api.connect) != 1 || api.connect[0] != (connectCall{id, true}) {
		t.Fatalf("connect = %+v, want a reboot resume", api.connect)
	}
}

func TestRestartPausedSandboxOnlyResumes(t *testing.T) {
	t.Parallel()

	p, api, _ := newProvider(nil, 0)
	api.boxes[testBox].State = "paused"

	if err := p.Restart(context.Background(), testBox); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if len(api.paused) != 0 || len(api.connect) != 1 || !api.connect[0].reboot {
		t.Fatalf("paused = %+v connect = %+v", api.paused, api.connect)
	}
}

func TestRestartErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(*fakeAPI)
		id    sandbox.ID
	}{
		{name: "unknown sandbox", setup: func(*fakeAPI) {}, id: "missing"},
		{name: "bad name", setup: func(*fakeAPI) {}, id: "Bad_Name"},
		{name: "pause fails", setup: func(a *fakeAPI) { a.pauseErr = errors.New("pause failed for " + testKey) }, id: testBox},
		{name: "resume fails", setup: func(a *fakeAPI) { a.connectErr = errors.New("resume failed for " + testKey) }, id: testBox},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, api, _ := newProvider(nil, 0)
			tt.setup(api)
			err := p.Restart(context.Background(), tt.id)
			if err == nil || strings.Contains(err.Error(), testKey) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
