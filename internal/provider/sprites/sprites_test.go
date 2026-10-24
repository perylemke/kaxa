package sprites_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/sbox/internal/provider/sprites"
	"github.com/perylemke/sbox/internal/sandbox"
)

type fakeSDKClient struct {
	create  func(ctx context.Context, name string) (*spritesdk.Sprite, error)
	list    func(ctx context.Context, prefix string) ([]*spritesdk.Sprite, error)
	destroy func(ctx context.Context, name string) error
	sprite  func(name string) *spritesdk.Sprite
}

func (f *fakeSDKClient) CreateSprite(ctx context.Context, name string) (*spritesdk.Sprite, error) {
	if f.create != nil {
		return f.create(ctx, name)
	}
	return nil, errors.New("create not stubbed")
}

func (f *fakeSDKClient) ListAllSprites(ctx context.Context, prefix string) ([]*spritesdk.Sprite, error) {
	if f.list != nil {
		return f.list(ctx, prefix)
	}
	return nil, nil
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

	_, err := p.Create(ctx, sandbox.Spec{Name: "demo"})
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
		create: func(_ context.Context, _ string) (*spritesdk.Sprite, error) {
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
