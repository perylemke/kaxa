package sprites_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/sbox/internal/provider/sprites"
	"github.com/perylemke/sbox/internal/sandbox"
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
	})
	if err != nil {
		t.Fatalf("PublishAPI: %v", err)
	}
	want := []string{
		"sprite-env", "services", "create", "sbox-api",
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
			case "uname":
				_, _ = io.WriteString(stdout, "x86_64\n")
			case "sha256sum":
				_, _ = io.WriteString(stdout, "bb69984aab06d9463ed5da46a81b71d2d1582ba75c9c07c65f49fecbb52f7b7b  /tmp/sbox-webtmux.tar.gz\n")
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
