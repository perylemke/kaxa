package sprites_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/kaxa/internal/provider/sprites"
	"github.com/perylemke/kaxa/internal/sandbox"
)

// Commands the fake remote runner recognizes.
const (
	cmdUname     = "uname"
	cmdSha256sum = "sha256sum"
)

func TestPublishAPIBuildsServiceCommand(t *testing.T) {
	t.Parallel()

	var got []string
	client := &fakeSDKClient{
		run: func(_ context.Context, _ string, argv []string, _ io.Reader, stdout, _ io.Writer) error {
			if len(argv) >= 3 && argv[2] == "get" {
				_, _ = io.WriteString(stdout, `{"state":{"status":"running"}}`)
				return nil
			}
			got = append([]string(nil), argv...)
			return nil
		},
	}
	p := sprites.NewWithClient(client, "token")
	err := p.PublishAPI(context.Background(), "box", "/home/sprite/workspace/api-python", 8000, []string{
		".venv/bin/uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000",
	}, nil)
	if err != nil {
		t.Fatalf("PublishAPI: %v", err)
	}
	want := []string{
		"sprite-env", "services", "create", "kaxa-api",
		"--cmd", ".venv/bin/uvicorn",
		"--args", "app.main:app,--host,0.0.0.0,--port,8000",
		"--dir", "/home/sprite/workspace/api-python",
		"--http-port", "8000",
	}
	if len(got) < len(want)+2 || !reflect.DeepEqual(got[:len(want)], want) {
		t.Fatalf("argv = %#v", got)
	}
	if got[len(want)] != "--env" || !strings.Contains(got[len(want)+1], "HOME=/home/sprite") || !strings.Contains(got[len(want)+1], "USER=sprite") {
		t.Fatalf("env = %#v", got[len(want):])
	}
	if got[len(got)-1] != "--no-stream" {
		t.Fatalf("argv = %#v", got)
	}
}

func TestPublishAPIAppendsServiceEnv(t *testing.T) {
	t.Parallel()

	var got []string
	client := &fakeSDKClient{
		run: func(_ context.Context, _ string, argv []string, _ io.Reader, stdout, _ io.Writer) error {
			if len(argv) >= 3 && argv[2] == "get" {
				_, _ = io.WriteString(stdout, `{"state":{"status":"running"}}`)
				return nil
			}
			got = append([]string(nil), argv...)
			return nil
		},
	}
	p := sprites.NewWithClient(client, "token")
	err := p.PublishAPI(context.Background(), "box", "/home/sprite", 8000, []string{"app"}, []string{
		"DATABASE_URL=postgres://u:p@db:5432/app", "REDIS_URL=redis://cache:6379/0",
	})
	if err != nil {
		t.Fatalf("PublishAPI: %v", err)
	}
	idx := slices.Index(got, "--env")
	if idx < 0 || idx+1 >= len(got) {
		t.Fatalf("argv = %#v", got)
	}
	env := strings.Split(got[idx+1], ",")
	if !slices.Contains(env, "USER=sprite") || !slices.Contains(env, "DATABASE_URL=postgres://u:p@db:5432/app") || !slices.Contains(env, "REDIS_URL=redis://cache:6379/0") {
		t.Fatalf("env = %#v", env)
	}
	if slices.Index(env, "DATABASE_URL=postgres://u:p@db:5432/app") < slices.Index(env, "USER=sprite") {
		t.Fatalf("service env must follow the tool env: %#v", env)
	}
}

func TestPublishAPIRejectsBadEnv(t *testing.T) {
	t.Parallel()

	p := sprites.NewWithClient(&fakeSDKClient{}, "token")
	for _, env := range [][]string{{"NOEQUALS"}, {"A=b,c"}, {"1X=y"}} {
		if err := p.PublishAPI(context.Background(), "box", "/home/sprite", 8000, []string{"app"}, env); err == nil {
			t.Fatalf("env %v accepted", env)
		}
	}
}

func TestCreateRunningWithAPILeavesURLForAPI(t *testing.T) {
	t.Parallel()

	var runCalls [][]string
	client := &fakeSDKClient{
		create: func(context.Context, string, []string) (*spritesdk.Sprite, error) {
			return &spritesdk.Sprite{}, nil
		},
		run: func(_ context.Context, _ string, argv []string, _ io.Reader, stdout, _ io.Writer) error {
			runCalls = append(runCalls, append([]string(nil), argv...))
			switch argv[0] {
			case cmdUname:
				_, _ = io.WriteString(stdout, "x86_64\n")
			case cmdSha256sum:
				_, _ = io.WriteString(stdout, "bb69984aab06d9463ed5da46a81b71d2d1582ba75c9c07c65f49fecbb52f7b7b  /tmp/kaxa-webtmux.tar.gz\n")
			}
			return nil
		},
	}
	p := sprites.NewWithClient(client, "token")
	_, err := p.Create(context.Background(), sandbox.Spec{Name: spriteName, Running: true, ExposeAPI: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var sawWebtmux, sawHTTPPort bool
	for _, argv := range runCalls {
		if reflect.DeepEqual(argv, sprites.RunningWebtmuxArgv) {
			sawWebtmux = true
		}
		if reflect.DeepEqual(argv, sprites.RunningHTTPServiceArgv) {
			sawHTTPPort = true
		}
	}
	if !sawWebtmux || sawHTTPPort {
		t.Fatalf("webtmux=%v httpPort=%v calls=%v", sawWebtmux, sawHTTPPort, runCalls)
	}
}

func TestSetURLAuthPutsAuth(t *testing.T) {
	t.Parallel()

	const token = "public-test-token" // #nosec G101 -- test fixture
	var body map[string]map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/sprites/"+spriteName {
			writeSpriteDetail(w)
			return
		}
		if r.Method != http.MethodPut || r.URL.Path != "/v1/sprites/"+spriteName {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	p := sprites.New()
	if err := p.ConfigureWithOptions(token, srv.URL, spritesdk.WithDisableControl()); err != nil {
		t.Fatalf("configure: %v", err)
	}
	for _, auth := range []string{sandbox.URLAuthPublic, sandbox.URLAuthSprite} {
		body = nil
		if err := p.SetURLAuth(context.Background(), spriteName, auth); err != nil {
			t.Fatalf("SetURLAuth(%s): %v", auth, err)
		}
		if body["url_settings"]["auth"] != auth {
			t.Fatalf("auth %s body = %#v", auth, body)
		}
	}
}

func TestSetURLAuthRejectsUnknown(t *testing.T) {
	t.Parallel()

	p := sprites.NewWithClient(&fakeSDKClient{}, "token")
	err := p.SetURLAuth(context.Background(), spriteName, "open")
	if err == nil || !strings.Contains(err.Error(), "invalid url auth") {
		t.Fatalf("err = %v", err)
	}
}

func TestSetURLAuthRefusesPublicTerminal(t *testing.T) {
	t.Parallel()

	client := &fakeSDKClient{
		get: func(context.Context, string) (*spritesdk.Sprite, error) {
			return &spritesdk.Sprite{Labels: []string{"kaxa", sandbox.TerminalLabel}}, nil
		},
	}
	p := sprites.NewWithClient(client, "token")
	err := p.SetURLAuth(context.Background(), spriteName, sandbox.URLAuthPublic)
	if err == nil || !strings.Contains(err.Error(), "unauthenticated tmux terminal") {
		t.Fatalf("err = %v", err)
	}
}

func TestSetURLAuthSpriteAllowedForTerminal(t *testing.T) {
	t.Parallel()

	client := &fakeSDKClient{
		get: func(context.Context, string) (*spritesdk.Sprite, error) {
			return &spritesdk.Sprite{Labels: []string{sandbox.TerminalLabel}}, nil
		},
	}
	p := sprites.NewWithClient(client, "token")
	err := p.SetURLAuth(context.Background(), spriteName, sandbox.URLAuthSprite)
	if err != nil && strings.Contains(err.Error(), "unauthenticated") {
		t.Fatalf("sprite auth refused: %v", err)
	}
}

func TestCreateRunningWithoutAPILabelsTerminal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		spec sandbox.Spec
		want bool
	}{
		{name: "running", spec: sandbox.Spec{Name: spriteName, Running: true}, want: true},
		{name: "running with api", spec: sandbox.Spec{Name: spriteName, Running: true, ExposeAPI: true}},
		{name: "plain", spec: sandbox.Spec{Name: spriteName}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var labels []string
			client := &fakeSDKClient{
				create: func(_ context.Context, _ string, l []string) (*spritesdk.Sprite, error) {
					labels = append([]string(nil), l...)
					return &spritesdk.Sprite{}, nil
				},
				run: func(_ context.Context, _ string, argv []string, _ io.Reader, stdout, _ io.Writer) error {
					switch argv[0] {
					case cmdUname:
						_, _ = io.WriteString(stdout, "x86_64\n")
					case cmdSha256sum:
						_, _ = io.WriteString(stdout, "bb69984aab06d9463ed5da46a81b71d2d1582ba75c9c07c65f49fecbb52f7b7b  /tmp/kaxa-webtmux.tar.gz\n")
					}
					return nil
				},
			}
			p := sprites.NewWithClient(client, "token")
			if _, err := p.Create(context.Background(), tt.spec); err != nil {
				t.Fatalf("create: %v", err)
			}
			if got := slices.Contains(labels, sandbox.TerminalLabel); got != tt.want {
				t.Fatalf("labels = %v, terminal label = %v, want %v", labels, got, tt.want)
			}
		})
	}
}

func TestPublishAPIRejectsOptionAsBinary(t *testing.T) {
	t.Parallel()

	p := sprites.NewWithClient(&fakeSDKClient{}, "token")
	err := p.PublishAPI(context.Background(), "box", "/home/sprite", 8000, []string{"--http-port", "1"}, nil)
	if err == nil || !strings.Contains(err.Error(), "must start with a binary") {
		t.Fatalf("err = %v", err)
	}
}
