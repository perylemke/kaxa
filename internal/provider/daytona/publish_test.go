package daytona

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
)

var serviceArgv = []string{"python", "-m", "http.server", "8000"}

func isService(req runSpec) bool { return req.argv[0] == "python" }

func publishFixture(t *testing.T, h func(runSpec, string) fakeResult) (*Provider, *fakeAPI, *fakeToolbox) {
	t.Helper()
	tb := newFakeToolbox(h)
	p, api := newTestProvider(t, tb)
	if _, err := p.Create(t.Context(), sandbox.Spec{Name: "api", ExposeAPI: true, APIPort: 8000}); err != nil {
		t.Fatal(err)
	}
	return p, api, tb
}

func TestPublishAPIStartsServiceAndExposesPort(t *testing.T) {
	t.Parallel()
	p, api, tb := publishFixture(t, func(req runSpec, _ string) fakeResult {
		return fakeResult{running: isService(req)}
	})
	err := p.PublishAPI(t.Context(), "api", daytonaHome, 8000, serviceArgv, []string{"MODE=prod"})
	if err != nil {
		t.Fatalf("PublishAPI: %v", err)
	}
	svc := tb.reqs[0]
	if svc.dir != daytonaHome || !slices.Contains(svc.env, "MODE=prod") {
		t.Fatalf("service request = %+v", svc)
	}
	if !api.has("setport sb-1 8000") {
		t.Fatalf("port not exposed: %v", api.calls)
	}
	d, err := p.Inspect(t.Context(), "api")
	if err != nil || d.URL == "" || d.Auth != sandbox.URLAuthPublic {
		t.Fatalf("Inspect = %+v, %v", d, err)
	}
	if probe := tb.reqs[1].argv; probe[0] != bashCmd || probe[len(probe)-1] != "8000" {
		t.Fatalf("probe = %v", probe)
	}
}

func TestPublishAPIServiceExitsEarly(t *testing.T) {
	t.Parallel()
	p, api, _ := publishFixture(t, func(req runSpec, _ string) fakeResult {
		if isService(req) {
			return fakeResult{code: 1, stdout: "address in use"}
		}
		return fakeResult{}
	})
	err := p.PublishAPI(t.Context(), "api", daytonaHome, 8000, serviceArgv, nil)
	if err == nil || !strings.Contains(err.Error(), "address in use") || !strings.Contains(err.Error(), "status 1") {
		t.Fatalf("err = %v", err)
	}
	if api.has("setport sb-1 8000") {
		t.Fatal("port exposed for a dead service")
	}
}

func TestPublishAPIServiceNeverListens(t *testing.T) {
	t.Parallel()
	p, api, _ := publishFixture(t, func(req runSpec, _ string) fakeResult {
		if isService(req) {
			return fakeResult{running: true}
		}
		return fakeResult{code: 1}
	})
	p.SetReadiness(30*time.Millisecond, time.Millisecond)
	err := p.PublishAPI(t.Context(), "api", daytonaHome, 8000, serviceArgv, nil)
	if err == nil || !strings.Contains(err.Error(), "did not start listening") {
		t.Fatalf("err = %v", err)
	}
	if api.has("setport sb-1 8000") {
		t.Fatal("port exposed for a silent service")
	}
}

func TestPublishAPIPortMustMatchCreate(t *testing.T) {
	t.Parallel()
	p, api, _ := publishFixture(t, okHandler)
	if err := p.PublishAPI(t.Context(), "api", daytonaHome, 9000, serviceArgv, nil); err == nil {
		t.Fatal("different port accepted")
	}
	api.add("plain", statusRunning)
	if err := p.PublishAPI(t.Context(), "plain", daytonaHome, 8000, serviceArgv, nil); err == nil {
		t.Fatal("sandbox without an API accepted")
	}
}

func TestPublishAPIValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		dir  string
		port int
		argv []string
		env  []string
	}{
		{"bad dir", "/etc", 8000, serviceArgv, nil},
		{"nested dir", workspaceDir + "/a/b", 8000, serviceArgv, nil},
		{"traversal dir", workspaceDir + "/..", 8000, serviceArgv, nil},
		{"port zero", daytonaHome, 0, serviceArgv, nil},
		{"port too big", daytonaHome, 70000, serviceArgv, nil},
		{"no argv", daytonaHome, 8000, nil, nil},
		{"flag first", daytonaHome, 8000, []string{"-m", "x"}, nil},
		{"empty arg", daytonaHome, 8000, []string{"python", ""}, nil},
		{"bad env key", daytonaHome, 8000, serviceArgv, []string{"1BAD=x"}},
		{"env newline", daytonaHome, 8000, serviceArgv, []string{"A=b\nc"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, _, tb := publishFixture(t, okHandler)
			if err := p.PublishAPI(t.Context(), "api", tt.dir, tt.port, tt.argv, tt.env); err == nil {
				t.Fatal("PublishAPI succeeded")
			}
			if len(tb.reqs) != 0 {
				t.Fatalf("commands ran: %v", tb.argvs())
			}
		})
	}
}

func TestRestart(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status string
		calls  []string
	}{
		{"running", statusRunning, []string{"stop sb-1", "start sb-1"}},
		{"stopped", statusStopped, []string{"start sb-1"}},
		{"paused", statusPaused, []string{"resume sb-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, api := newTestProvider(t, newFakeToolbox(okHandler))
			api.add(boxName, tt.status)
			api.calls = nil
			if err := p.Restart(t.Context(), boxName); err != nil {
				t.Fatalf("Restart: %v", err)
			}
			if !slices.Equal(api.calls, tt.calls) {
				t.Fatalf("calls = %v, want %v", api.calls, tt.calls)
			}
			if rec, _ := api.Find(t.Context(), boxName); rec.Status != statusRunning {
				t.Fatalf("status = %q", rec.Status)
			}
		})
	}
	p, _ := newTestProvider(t, newFakeToolbox(okHandler))
	if err := p.Restart(t.Context(), "missing"); err == nil {
		t.Fatal("missing sandbox restarted")
	}
}
