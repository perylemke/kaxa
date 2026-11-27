package sprites_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/kaxa/internal/provider/sprites"
	"github.com/perylemke/kaxa/internal/sandbox"
)

const (
	fixtureToken = "tok-secret-value" // #nosec G101 -- test fixture
	validWorkDir = "/home/sprite"
)

var errBoom = errors.New("boom")

type opFunc func(p *sprites.Provider, id sandbox.ID) error

func TestProviderSupportsRunning(t *testing.T) {
	t.Parallel()
	sprites.New().SupportsRunning()
}

func TestOperationsValidateAndFail(t *testing.T) {
	t.Parallel()

	ops := map[string]opFunc{
		"upgrade": func(p *sprites.Provider, id sandbox.ID) error { return p.Upgrade(context.Background(), id) },
		"destroy": func(p *sprites.Provider, id sandbox.ID) error { return p.Destroy(context.Background(), id) },
		"inspect": func(p *sprites.Provider, id sandbox.ID) error {
			_, err := p.Inspect(context.Background(), id)
			return err
		},
		"exec": func(p *sprites.Provider, id sandbox.ID) error {
			return p.Exec(context.Background(), id, sandbox.ExecOptions{Command: []string{"ls"}})
		},
		"prepare": func(p *sprites.Provider, id sandbox.ID) error { return p.PrepareWorkspace(context.Background(), id) },
		"sync": func(p *sprites.Provider, id sandbox.ID) error {
			return p.SyncWorkspace(context.Background(), id, strings.NewReader("x"))
		},
		"seturlauth": func(p *sprites.Provider, id sandbox.ID) error {
			return p.SetURLAuth(context.Background(), id, sandbox.URLAuthSprite)
		},
		"publish": func(p *sprites.Provider, id sandbox.ID) error {
			return p.PublishAPI(context.Background(), id, validWorkDir, 8000, []string{cmdApp}, nil)
		},
	}
	for name, op := range ops {
		t.Run(name+"/invalid name", func(t *testing.T) {
			t.Parallel()
			if err := op(sprites.NewWithClient(&fakeSDKClient{}, fixtureToken), "../evil"); err == nil {
				t.Fatal("expected error")
			}
		})
		t.Run(name+"/missing token", func(t *testing.T) {
			t.Parallel()
			if err := op(sprites.New(), spriteName); !errors.Is(err, sprites.ErrMissingToken) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestListResults(t *testing.T) {
	t.Parallel()

	t.Run("entries", func(t *testing.T) {
		t.Parallel()
		client := &fakeSDKClient{list: func(context.Context, string) ([]sprites.SpriteListEntry, error) {
			return []sprites.SpriteListEntry{{Name: "a", Status: "warm", Labels: []string{"x"}}}, nil
		}}
		got, err := sprites.NewWithClient(client, fixtureToken).List(context.Background())
		if err != nil || len(got) != 1 || got[0].ID != "a" || got[0].Provider != "sprites" {
			t.Fatalf("got %+v err %v", got, err)
		}
	})
	t.Run("error is redacted", func(t *testing.T) {
		t.Parallel()
		client := &fakeSDKClient{list: func(context.Context, string) ([]sprites.SpriteListEntry, error) {
			return nil, errors.New("bad " + fixtureToken)
		}}
		_, err := sprites.NewWithClient(client, fixtureToken).List(context.Background())
		if err == nil || strings.Contains(err.Error(), fixtureToken) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("missing token", func(t *testing.T) {
		t.Parallel()
		if _, err := sprites.New().List(context.Background()); !errors.Is(err, sprites.ErrMissingToken) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestUpgradeDestroyClientErrors(t *testing.T) {
	t.Parallel()

	client := &fakeSDKClient{
		upgrade: func(context.Context, string) error { return errors.New("up " + fixtureToken) },
		destroy: func(context.Context, string) error { return errors.New("rm " + fixtureToken) },
	}
	p := sprites.NewWithClient(client, fixtureToken)
	for name, err := range map[string]error{
		"upgrade": p.Upgrade(context.Background(), spriteName),
		"destroy": p.Destroy(context.Background(), spriteName),
	} {
		if err == nil || strings.Contains(err.Error(), fixtureToken) {
			t.Fatalf("%s err = %v", name, err)
		}
	}
	if err := sprites.NewWithClient(&fakeSDKClient{}, fixtureToken).Upgrade(context.Background(), spriteName); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
}

func TestExecEarlyErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tests := []struct {
		name   string
		client *fakeSDKClient
		opts   sandbox.ExecOptions
	}{
		{"empty command", &fakeSDKClient{}, sandbox.ExecOptions{}},
		{"get fails", &fakeSDKClient{get: func(context.Context, string) (*spritesdk.Sprite, error) {
			return nil, errors.New("get " + fixtureToken)
		}}, sandbox.ExecOptions{Command: []string{"ls"}}},
		{"bad dir", &fakeSDKClient{sprite: func(name string) *spritesdk.Sprite {
			return spritesdk.New(fixtureToken).Sprite(name)
		}}, sandbox.ExecOptions{Command: []string{"ls"}, Dir: "relative"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := sprites.NewWithClient(tt.client, fixtureToken).Exec(ctx, spriteName, tt.opts)
			if err == nil {
				t.Fatal("expected error")
			}
			if strings.Contains(err.Error(), fixtureToken) {
				t.Fatalf("leaked token: %v", err)
			}
		})
	}
}

func TestExecEnvReadError(t *testing.T) {
	t.Parallel()

	client := &fakeSDKClient{}
	client.envFile = "A=1\n"
	failing := &failingEnvClient{fakeSDKClient: client}
	err := sprites.NewWithClient(failing, fixtureToken).Exec(context.Background(), spriteName, sandbox.ExecOptions{Command: []string{"ls"}})
	if err == nil {
		t.Fatal("expected error")
	}
}

// failingEnvClient fails every RunIn, so the env read cannot succeed.
type failingEnvClient struct{ *fakeSDKClient }

func (f *failingEnvClient) RunIn(context.Context, string, string, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return errors.New("run " + fixtureToken)
}

func TestPublishAPIValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		workDir string
		port    int
		argv    []string
		env     []string
	}{
		{"bad workdir", "/etc", 8000, []string{cmdApp}, nil},
		{"port low", validWorkDir, 0, []string{cmdApp}, nil},
		{"port high", validWorkDir, 70000, []string{cmdApp}, nil},
		{"empty argv", validWorkDir, 8000, nil, nil},
		{"flag first", validWorkDir, 8000, []string{"--x"}, nil},
		{"empty arg", validWorkDir, 8000, []string{cmdApp, ""}, nil},
		{"comma arg", validWorkDir, 8000, []string{cmdApp, "a,b"}, nil},
		{"bad env", validWorkDir, 8000, []string{cmdApp}, []string{"NOEQUALS"}},
		{"env comma", validWorkDir, 8000, []string{cmdApp}, []string{"A=b,c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := sprites.NewWithClient(&fakeSDKClient{}, fixtureToken)
			if err := p.PublishAPI(context.Background(), spriteName, tt.workDir, tt.port, tt.argv, tt.env); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestPublishAPIRemoteFailures(t *testing.T) {
	t.Parallel()

	getBody := func(body string) *fakeSDKClient {
		return &fakeSDKClient{run: func(_ context.Context, _ string, argv []string, _ io.Reader, stdout, _ io.Writer) error {
			if len(argv) >= 3 && argv[2] == cmdGet {
				_, _ = io.WriteString(stdout, body)
			}
			return nil
		}}
	}
	tests := []struct {
		name   string
		client sandboxClient
	}{
		{"service failed with detail", getBody(`{"state":{"status":"failed","error":"port in use"}}`)},
		{"service failed", getBody(`{"state":{"status":"failed"}}`)},
		{"create fails", &fakeSDKClient{run: func(context.Context, string, []string, io.Reader, io.Writer, io.Writer) error {
			return errBoom
		}}},
		{"status read fails", &fakeSDKClient{run: func(_ context.Context, _ string, argv []string, _ io.Reader, _, _ io.Writer) error {
			if len(argv) >= 3 && argv[2] == cmdGet {
				return errBoom
			}
			return nil
		}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := sprites.NewWithClient(tt.client, fixtureToken)
			if err := p.PublishAPI(context.Background(), spriteName, validWorkDir, 8000, []string{cmdApp}, nil); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestPublishAPICanceledWhileWaiting(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &fakeSDKClient{}
	p := sprites.NewWithClient(client, fixtureToken)
	if err := p.PublishAPI(ctx, spriteName, "/home/sprite/workspace/repo", 8000, []string{cmdApp}, []string{"A=1"}); err == nil {
		t.Fatal("expected error")
	}
}

// sandboxClient is the client the fake satisfies; it keeps the table typed.
type sandboxClient = *fakeSDKClient

func TestSetURLAuthPaths(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	t.Run("invalid auth", func(t *testing.T) {
		t.Parallel()
		if err := sprites.NewWithClient(&fakeSDKClient{}, fixtureToken).SetURLAuth(ctx, spriteName, "nope"); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("public refused for terminal", func(t *testing.T) {
		t.Parallel()
		client := &fakeSDKClient{get: func(context.Context, string) (*spritesdk.Sprite, error) {
			return &spritesdk.Sprite{Labels: []string{sandbox.TerminalLabel}}, nil
		}}
		if err := sprites.NewWithClient(client, fixtureToken).SetURLAuth(ctx, spriteName, sandbox.URLAuthPublic); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("get fails", func(t *testing.T) {
		t.Parallel()
		client := &fakeSDKClient{get: func(context.Context, string) (*spritesdk.Sprite, error) {
			return nil, errors.New("get " + fixtureToken)
		}}
		err := sprites.NewWithClient(client, fixtureToken).SetURLAuth(ctx, spriteName, sandbox.URLAuthPublic)
		if err == nil || strings.Contains(err.Error(), fixtureToken) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("fake client is not the API", func(t *testing.T) {
		t.Parallel()
		if err := sprites.NewWithClient(&fakeSDKClient{}, fixtureToken).SetURLAuth(ctx, spriteName, sandbox.URLAuthSprite); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestCreateRunningStepFailures(t *testing.T) {
	t.Parallel()

	const goodSum = "bb69984aab06d9463ed5da46a81b71d2d1582ba75c9c07c65f49fecbb52f7b7b  /tmp/kaxa-webtmux.tar.gz\n"
	tests := []struct {
		name   string
		failOn string
		uname  string
		expose bool
	}{
		{"hold", "/v1/tasks", archX86, false},
		{"uname", cmdUname, archX86, false},
		{"unsupported arch", "", "riscv\n", false},
		{"mkdir", cmdMkdir, archX86, false},
		{"download", "curl", "aarch64\n", false},
		{"checksum", cmdSha256sum, archX86, false},
		{"extract", "tar", archX86, false},
		{"move", "mv", "arm64\n", false},
		{"chmod", cmdChmod, archX86, false},
		{"cleanup", "rm", archX86, false},
		{"service", cmdCreate, archX86, false},
		{"service api owns url", cmdCreate, archX86, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := &fakeSDKClient{
				create: func(context.Context, string, []string) (*spritesdk.Sprite, error) { return &spritesdk.Sprite{}, nil },
				run: func(_ context.Context, _ string, argv []string, _ io.Reader, stdout, stderr io.Writer) error {
					switch argv[0] {
					case cmdUname:
						if tt.failOn == cmdUname {
							return errBoom
						}
						_, _ = io.WriteString(stdout, tt.uname)
						return nil
					case cmdSha256sum:
						_, _ = io.WriteString(stdout, goodSum)
					}
					if tt.failOn != "" && strings.Contains(strings.Join(argv, " "), tt.failOn) {
						_, _ = io.WriteString(stderr, "detail")
						return errors.New("fail " + fixtureToken)
					}
					return nil
				},
			}
			_, err := sprites.NewWithClient(client, fixtureToken).Create(context.Background(), sandbox.Spec{Name: spriteName, Running: true, ExposeAPI: tt.expose})
			if err == nil {
				t.Fatal("expected error")
			}
			if strings.Contains(err.Error(), fixtureToken) {
				t.Fatalf("leaked token: %v", err)
			}
		})
	}
}

func TestPrepareAndSyncWorkspace(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	t.Run("prepare ok", func(t *testing.T) {
		t.Parallel()
		if err := sprites.NewWithClient(&fakeSDKClient{}, fixtureToken).PrepareWorkspace(ctx, spriteName); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("prepare fails", func(t *testing.T) {
		t.Parallel()
		client := &fakeSDKClient{run: func(context.Context, string, []string, io.Reader, io.Writer, io.Writer) error { return errBoom }}
		if err := sprites.NewWithClient(client, fixtureToken).PrepareWorkspace(ctx, spriteName); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("sync ok", func(t *testing.T) {
		t.Parallel()
		var got [][]string
		client := &fakeSDKClient{run: func(_ context.Context, _ string, argv []string, stdin io.Reader, _, _ io.Writer) error {
			got = append(got, argv)
			if stdin != nil {
				_, _ = io.Copy(io.Discard, stdin)
			}
			return nil
		}}
		if err := sprites.NewWithClient(client, fixtureToken).SyncWorkspace(ctx, spriteName, strings.NewReader("x")); err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Fatalf("calls = %v", got)
		}
	})
	t.Run("sync nil archive", func(t *testing.T) {
		t.Parallel()
		if err := sprites.NewWithClient(&fakeSDKClient{}, fixtureToken).SyncWorkspace(ctx, spriteName, nil); err == nil {
			t.Fatal("expected error")
		}
	})
	for _, failAt := range []string{cmdMkdir, cmdTar} {
		t.Run("sync fails at "+failAt, func(t *testing.T) {
			t.Parallel()
			client := &fakeSDKClient{run: func(_ context.Context, _ string, argv []string, _ io.Reader, _, _ io.Writer) error {
				if argv[0] == failAt {
					return errBoom
				}
				return nil
			}}
			if err := sprites.NewWithClient(client, fixtureToken).SyncWorkspace(ctx, spriteName, strings.NewReader("x")); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

const (
	cmdMkdir  = "mkdir"
	cmdTar    = "tar"
	cmdChmod  = "chmod"
	cmdCreate = "create"
	cmdGet    = "get"
	cmdApp    = "app"
	archX86   = "x86_64\n"
)

func TestInspectResults(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tests := []struct {
		name     string
		settings *spritesdk.URLSettings
		want     string
	}{
		{"nil", nil, ""},
		{"empty", &spritesdk.URLSettings{}, ""},
		{"auth only", &spritesdk.URLSettings{Auth: "public"}, "public"},
		{"both", &spritesdk.URLSettings{Auth: "sprite", PrivateAccess: "org"}, "sprite · org"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := &fakeSDKClient{get: func(context.Context, string) (*spritesdk.Sprite, error) {
				return &spritesdk.Sprite{URLSettings: tt.settings}, nil
			}}
			d, err := sprites.NewWithClient(client, fixtureToken).Inspect(ctx, spriteName)
			if err != nil {
				t.Fatal(err)
			}
			if d.Auth != tt.want {
				t.Fatalf("auth = %q", d.Auth)
			}
		})
	}
	t.Run("get fails", func(t *testing.T) {
		t.Parallel()
		client := &fakeSDKClient{get: func(context.Context, string) (*spritesdk.Sprite, error) {
			return nil, errors.New("x " + fixtureToken)
		}}
		_, err := sprites.NewWithClient(client, fixtureToken).Inspect(ctx, spriteName)
		if err == nil || strings.Contains(err.Error(), fixtureToken) {
			t.Fatalf("err = %v", err)
		}
	})
}
