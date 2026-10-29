package sprites_test

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/kaxa/internal/provider/sprites"
	"github.com/perylemke/kaxa/internal/sandbox"
)

type fakeSDKClient struct {
	create  func(ctx context.Context, name string, labels []string) (*spritesdk.Sprite, error)
	get     func(ctx context.Context, name string) (*spritesdk.Sprite, error)
	list    func(ctx context.Context, prefix string) ([]sprites.SpriteListEntry, error)
	upgrade func(ctx context.Context, name string) error
	restart func(ctx context.Context, name string) error
	run     func(ctx context.Context, name string, argv []string, stdin io.Reader, stdout, stderr io.Writer) error
	destroy func(ctx context.Context, name string) error
	sprite  func(name string) *spritesdk.Sprite
}

func (f *fakeSDKClient) CreateSprite(ctx context.Context, name string, labels []string) (*spritesdk.Sprite, error) {
	if f.create != nil {
		return f.create(ctx, name, labels)
	}
	return nil, errors.New("create not stubbed")
}

func (f *fakeSDKClient) GetSprite(ctx context.Context, name string) (*spritesdk.Sprite, error) {
	if f.get != nil {
		return f.get(ctx, name)
	}
	return &spritesdk.Sprite{Status: spriteStatusWarm}, nil
}

func (f *fakeSDKClient) ListEntries(ctx context.Context, prefix string) ([]sprites.SpriteListEntry, error) {
	if f.list != nil {
		return f.list(ctx, prefix)
	}
	return nil, nil
}

func (f *fakeSDKClient) UpgradeSprite(ctx context.Context, name string) error {
	if f.upgrade != nil {
		return f.upgrade(ctx, name)
	}
	return nil
}

func (f *fakeSDKClient) RestartSprite(ctx context.Context, name string) error {
	if f.restart != nil {
		return f.restart(ctx, name)
	}
	return nil
}

func (f *fakeSDKClient) Run(ctx context.Context, name string, argv []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return f.RunIn(ctx, name, "", nil, argv, stdin, stdout, stderr)
}

func (f *fakeSDKClient) RunIn(ctx context.Context, name, _ string, _, argv []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if f.run != nil {
		return f.run(ctx, name, argv, stdin, stdout, stderr)
	}
	return nil
}

func (f *fakeSDKClient) DestroySprite(ctx context.Context, name string) error {
	if f.destroy != nil {
		return f.destroy(ctx, name)
	}
	return nil
}

func (f *fakeSDKClient) Sprite(name string) *spritesdk.Sprite {
	if f.sprite != nil {
		return f.sprite(name)
	}
	return nil
}

func (f *fakeSDKClient) Close() error { return nil }

func TestMissingToken(t *testing.T) {
	t.Parallel()

	p := sprites.New()
	ctx := context.Background()

	if err := p.Configure(""); !errors.Is(err, sprites.ErrMissingToken) {
		t.Fatalf("Configure empty: %v", err)
	}

	_, err := p.Create(ctx, sandbox.Spec{Name: spriteName})
	if !errors.Is(err, sprites.ErrMissingToken) {
		t.Fatalf("Create: %v", err)
	}
}

func TestInvalidName(t *testing.T) {
	t.Parallel()

	p := sprites.NewWithClient(&fakeSDKClient{}, "token")
	ctx := context.Background()

	_, err := p.Create(ctx, sandbox.Spec{Name: "../evil"})
	if err == nil {
		t.Fatal("expected invalid name error")
	}
}

func TestDestroyCallsClient(t *testing.T) {
	t.Parallel()

	var destroyed string
	client := &fakeSDKClient{
		destroy: func(_ context.Context, name string) error {
			destroyed = name
			return nil
		},
	}
	p := sprites.NewWithClient(client, "token")
	if err := p.Destroy(context.Background(), "my-sprite"); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if destroyed != "my-sprite" {
		t.Fatalf("destroyed = %q", destroyed)
	}
}

func TestRedactTokenInError(t *testing.T) {
	t.Parallel()

	const token = "leak-me-please" // #nosec G101 -- test fixture
	client := &fakeSDKClient{
		create: func(_ context.Context, _ string, _ []string) (*spritesdk.Sprite, error) {
			return nil, errors.New("auth failed: " + token)
		},
	}
	p := sprites.NewWithClient(client, token)
	_, err := p.Create(context.Background(), sandbox.Spec{Name: "ok"})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("error leaked token: %v", err)
	}
}

func TestCreateRunningRegistersHoldAndHTTPService(t *testing.T) {
	t.Parallel()

	var runCalls [][]string
	client := &fakeSDKClient{
		create: func(_ context.Context, _ string, _ []string) (*spritesdk.Sprite, error) {
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
	_, err := p.Create(context.Background(), sandbox.Spec{Name: spriteName, Running: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	assertRunningCalls(t, runCalls)
}

func assertRunningCalls(t *testing.T, runCalls [][]string) {
	t.Helper()
	if len(runCalls) == 0 || !reflect.DeepEqual(runCalls[0], sprites.RunningHoldArgv) {
		t.Fatalf("first run = %v", runCalls)
	}
	var sawDownload, sawService bool
	for _, argv := range runCalls {
		joined := strings.Join(argv, " ")
		if argv[0] == "curl" && strings.Contains(joined, "0612eeb71406c83603044fcb8e351a12e864afd5") && strings.Contains(joined, "webtmux-linux-amd64.tar.gz") {
			sawDownload = true
		}
		if reflect.DeepEqual(argv, sprites.RunningHTTPServiceArgv) {
			sawService = true
		}
		if strings.Contains(joined, "http.server") {
			t.Fatalf("still starting python http.server: %v", argv)
		}
	}
	if !sawDownload || !sawService {
		t.Fatalf("download=%v service=%v calls=%v", sawDownload, sawService, runCalls)
	}
}

func TestCreateRunningRejectsBadWebtmuxChecksum(t *testing.T) {
	t.Parallel()

	var startedService bool
	client := &fakeSDKClient{
		create: func(_ context.Context, _ string, _ []string) (*spritesdk.Sprite, error) {
			return &spritesdk.Sprite{}, nil
		},
		run: func(_ context.Context, _ string, argv []string, _ io.Reader, stdout, _ io.Writer) error {
			if reflect.DeepEqual(argv, sprites.RunningHTTPServiceArgv) {
				startedService = true
			}
			switch argv[0] {
			case cmdUname:
				_, _ = io.WriteString(stdout, "x86_64\n")
			case cmdSha256sum:
				_, _ = io.WriteString(stdout, "0000000000000000000000000000000000000000000000000000000000000000  /tmp/kaxa-webtmux.tar.gz\n")
			}
			return nil
		},
	}
	p := sprites.NewWithClient(client, "token")
	_, err := p.Create(context.Background(), sandbox.Spec{Name: spriteName, Running: true})
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %v", err)
	}
	if startedService {
		t.Fatal("service started after a bad checksum")
	}
}

func TestCreateWithoutRunningSkipsHold(t *testing.T) {
	t.Parallel()

	var runCalls int
	client := &fakeSDKClient{
		create: func(_ context.Context, _ string, _ []string) (*spritesdk.Sprite, error) {
			return &spritesdk.Sprite{}, nil
		},
		run: func(context.Context, string, []string, io.Reader, io.Writer, io.Writer) error {
			runCalls++
			return nil
		},
	}
	p := sprites.NewWithClient(client, "token")
	if _, err := p.Create(context.Background(), sandbox.Spec{Name: spriteName}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if runCalls != 0 {
		t.Fatalf("runCalls = %d", runCalls)
	}
}

func TestConfigureBuildsClient(t *testing.T) {
	t.Parallel()

	p := sprites.New()
	if err := p.Configure("test-token"); err != nil {
		t.Fatalf("configure: %v", err)
	}

	var nilOpt spritesdk.Option
	if err := p.ConfigureWithOptions("test-token", "https://api.sprites.dev", nilOpt); err != nil {
		t.Fatalf("configure with nil option: %v", err)
	}
}
