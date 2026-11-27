package vercel

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/sandbox"
)

type projectStep = project.Step

const apiPort = 8000

var apiArgv = []string{"python", "-m", "http.server", "8000"}

func TestPublishAPIOnExistingRoute(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	createBox(t, p, func(s *sandbox.Spec) { s.ExposeAPI, s.APIPort = true, apiPort })
	err := p.PublishAPI(context.Background(), boxName, workspaceDir+"/app", apiPort, apiArgv, []string{"MODE=prod"})
	if err != nil {
		t.Fatal(err)
	}
	if f.calls("PATCH /v2/sandboxes/{name}") != 0 {
		t.Fatal("an existing route was patched")
	}
	svc := cmdOf(f, 0)
	if _, waits := svc["wait"]; waits || svc["cwd"] != workspaceDir+"/app" || svc["command"] != shellBash {
		t.Fatalf("service body = %v", svc)
	}
	if asMap(svc["env"])["MODE"] != "prod" {
		t.Fatalf("service env = %v", svc["env"])
	}
	args := asList(cmdOf(f, 1)["args"])
	if args[len(args)-1] != "8000" || args[len(args)-5] != "bash" {
		t.Fatalf("probe args = %v", args)
	}
}

func TestPublishAPIAddsPort(t *testing.T) {
	t.Parallel()
	p, f := newTestProvider(t)
	f.lagStop = 2
	createBox(t, p, func(s *sandbox.Spec) { s.Labels = []string{wrapperArg0, "sync"} })
	first := f.sandboxes[boxName].session
	if err := p.PublishAPI(context.Background(), boxName, vercelHome, apiPort, apiArgv, nil); err != nil {
		t.Fatal(err)
	}
	sb := f.sandboxes[boxName]
	if len(sb.ports) != 1 || sb.ports[0] != apiPort || sb.session == first || sb.status != statusRunning {
		t.Fatalf("sandbox = %+v", sb)
	}
	if sb.tags[tagAPIPort] != "8000" || sb.tags[tagLabels] != "kaxa,sync" || sb.tags[tagManaged] != tagTrue {
		t.Fatalf("tags = %v", sb.tags)
	}
	d, err := p.Inspect(context.Background(), boxName)
	if err != nil || d.URL == "" {
		t.Fatalf("Inspect() = %+v, %v", d, err)
	}
}

func TestPublishAPIRejects(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		dir  string
		port int
		argv []string
		env  []string
	}{
		{keyName, vercelHome, apiPort, apiArgv, nil},
		{"dir outside", "/etc", apiPort, apiArgv, nil},
		{"dir nested", workspaceDir + "/a/b", apiPort, apiArgv, nil},
		{"dir empty repo", workspaceDir + "/", apiPort, apiArgv, nil},
		{"dir dotdot", workspaceDir + "/..", apiPort, apiArgv, nil},
		{"port low", vercelHome, 80, apiArgv, nil},
		{"port high", vercelHome, 70000, apiArgv, nil},
		{"port reserved", vercelHome, reservedPort, apiArgv, nil},
		{"no argv", vercelHome, apiPort, nil, nil},
		{"flag first", vercelHome, apiPort, []string{"-x"}, nil},
		{"empty arg", vercelHome, apiPort, []string{"a", ""}, nil},
		{"env no equals", vercelHome, apiPort, apiArgv, []string{"X"}},
		{"env bad key", vercelHome, apiPort, apiArgv, []string{"1X=1"}},
		{"env newline", vercelHome, apiPort, apiArgv, []string{"X=a\nb"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, f := newTestProvider(t)
			createBox(t, p)
			id := sandbox.ID(boxName)
			if tt.name == "name" {
				id = "Bad Name"
			}
			if err := p.PublishAPI(context.Background(), id, tt.dir, tt.port, tt.argv, tt.env); err == nil {
				t.Fatal("PublishAPI() accepted the request")
			}
			if f.calls("POST /v2/sandboxes/sessions/{sid}/cmd") != 0 {
				t.Fatal("a rejected request reached the sandbox")
			}
		})
	}
}

func TestPublishAPIFailures(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	t.Run("missing sandbox", func(t *testing.T) {
		t.Parallel()
		p, _ := newTestProvider(t)
		if err := p.PublishAPI(ctx, "missing", vercelHome, apiPort, apiArgv, nil); err == nil {
			t.Fatal("missing sandbox accepted")
		}
	})
	t.Run("patch fails", func(t *testing.T) {
		t.Parallel()
		p, f := newTestProvider(t)
		createBox(t, p)
		f.fail2("PATCH /v2/sandboxes/{name}", 500)
		if err := p.PublishAPI(ctx, boxName, vercelHome, apiPort, apiArgv, nil); err == nil {
			t.Fatal("patch failure ignored")
		}
	})
	t.Run("start fails", func(t *testing.T) {
		t.Parallel()
		p, f := newTestProvider(t)
		createBox(t, p, func(s *sandbox.Spec) { s.ExposeAPI, s.APIPort = true, apiPort })
		f.fail2("POST /v2/sandboxes/sessions/{sid}/cmd", 500)
		if err := p.PublishAPI(ctx, boxName, vercelHome, apiPort, apiArgv, nil); err == nil || !strings.Contains(err.Error(), "start api service") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("never listens", func(t *testing.T) {
		t.Parallel()
		p, f := newTestProvider(t)
		p.SetReadiness(30*time.Millisecond, time.Millisecond)
		createBox(t, p, func(s *sandbox.Spec) { s.ExposeAPI, s.APIPort = true, apiPort })
		f.exec = func(*fakeCmd) fakeResult { return fakeResult{code: 1} }
		if err := p.PublishAPI(ctx, boxName, vercelHome, apiPort, apiArgv, nil); err == nil || !strings.Contains(err.Error(), "did not start listening") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("canceled while waiting", func(t *testing.T) {
		t.Parallel()
		p, f := newTestProvider(t)
		createBox(t, p, func(s *sandbox.Spec) { s.ExposeAPI, s.APIPort = true, apiPort })
		f.exec = func(*fakeCmd) fakeResult { return fakeResult{code: 1} }
		cctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		p.SetReadiness(time.Minute, 50*time.Millisecond)
		if err := p.PublishAPI(cctx, boxName, vercelHome, apiPort, apiArgv, nil); err == nil {
			t.Fatal("canceled wait returned no error")
		}
	})
}

func TestAddPortWithoutRoute(t *testing.T) {
	t.Parallel()
	// A client that never reports the route makes addPort fail cleanly.
	c := &noRouteClient{}
	p := NewWithClient(c, testToken)
	s := &session{api: c, rec: Record{Name: boxName, SessionID: "s", Status: statusRunning}}
	p.SetReadiness(time.Second, time.Millisecond)
	if _, err := p.addPort(context.Background(), s, apiPort); err == nil || !strings.Contains(err.Error(), "route") {
		t.Fatalf("addPort() error = %v", err)
	}
}

// noRouteClient is a stopped-then-resumed sandbox that has no routes.
type noRouteClient struct{ apiClient }

func (*noRouteClient) Get(_ context.Context, name string, resume bool) (Record, error) {
	status := statusStopped
	if resume {
		status = statusRunning
	}
	return Record{Name: name, Status: status, SessionID: "s2"}, nil
}

func (*noRouteClient) StopSession(context.Context, string) error { return nil }

func (*noRouteClient) Update(context.Context, string, map[string]any) error { return nil }
