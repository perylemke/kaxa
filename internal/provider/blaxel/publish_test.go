package blaxel

import (
	"slices"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

var serviceArgv = []string{"python", "-m", "http.server", "8000"}

func TestPublishAPIStartsServiceAndPreview(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
	if _, err := p.Create(t.Context(), sandbox.Spec{Name: apiName, ExposeAPI: true, APIPort: 8000}); err != nil {
		t.Fatal(err)
	}
	err := p.PublishAPI(t.Context(), apiName, blaxelHome, 8000, serviceArgv, []string{"MODE=prod"})
	if err != nil {
		t.Fatalf("PublishAPI: %v", err)
	}
	if !commandIs(api.argvs()[0], serviceArgv...) || api.dirs[0] != blaxelHome {
		t.Fatalf("command = %v dir = %q", api.argvs()[0], api.dirs[0])
	}
	if !slices.Contains(api.envs[0], "MODE=prod") || !slices.Contains(api.envs[0], "HOME="+blaxelHome) {
		t.Fatalf("env = %v", api.envs[0])
	}
	d, err := p.Inspect(t.Context(), apiName)
	if err != nil || d.URL == "" || d.Auth != sandbox.URLAuthPublic {
		t.Fatalf("Inspect = %+v, %v", d, err)
	}
}

func TestPublishAPIServiceExitsEarly(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	api.handler = func(spec runSpec) fakeResult {
		if spec.argv[0] == "python" {
			return fakeResult{code: 1}
		}
		return fakeResult{}
	}
	p := newTestProvider(t, api)
	if _, err := p.Create(t.Context(), sandbox.Spec{Name: apiName, ExposeAPI: true, APIPort: 8000}); err != nil {
		t.Fatal(err)
	}
	err := p.PublishAPI(t.Context(), apiName, blaxelHome, 8000, serviceArgv, nil)
	if err == nil || !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("err = %v", err)
	}
	d, inspectErr := p.Inspect(t.Context(), apiName)
	if inspectErr != nil || d.URL != "" {
		t.Fatalf("preview created for a dead service: %+v, %v", d, inspectErr)
	}
}

func TestPublishAPIPortMustMatchCreate(t *testing.T) {
	t.Parallel()
	api := newFakeAPI()
	p := newTestProvider(t, api)
	if _, err := p.Create(t.Context(), sandbox.Spec{Name: apiName, ExposeAPI: true, APIPort: 8000}); err != nil {
		t.Fatal(err)
	}
	if err := p.PublishAPI(t.Context(), apiName, blaxelHome, 9000, serviceArgv, nil); err == nil {
		t.Fatal("different port accepted")
	}
	api.add("plain", statusRunning)
	if err := p.PublishAPI(t.Context(), "plain", blaxelHome, 8000, serviceArgv, nil); err == nil {
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
		{"port zero", blaxelHome, 0, serviceArgv, nil},
		{"port too big", blaxelHome, 70000, serviceArgv, nil},
		{"reserved port", blaxelHome, 8080, serviceArgv, nil},
		{"no argv", blaxelHome, 8000, nil, nil},
		{"flag first", blaxelHome, 8000, []string{"-m", "x"}, nil},
		{"empty arg", blaxelHome, 8000, []string{"python", ""}, nil},
		{"bad env key", blaxelHome, 8000, serviceArgv, []string{"1BAD=x"}},
		{"env newline", blaxelHome, 8000, serviceArgv, []string{"A=b\nc"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			api := newFakeAPI()
			p := newTestProvider(t, api)
			if _, err := p.Create(t.Context(), sandbox.Spec{Name: apiName, ExposeAPI: true, APIPort: 8000}); err != nil {
				t.Fatal(err)
			}
			before := len(api.argvs())
			if err := p.PublishAPI(t.Context(), apiName, tt.dir, tt.port, tt.argv, tt.env); err == nil {
				t.Fatal("PublishAPI succeeded")
			}
			if len(api.argvs()) != before {
				t.Fatalf("commands ran: %v", api.argvs()[before:])
			}
		})
	}
}
