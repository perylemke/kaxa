package sprites

import (
	"context"

	spritesdk "github.com/superfly/sprites-go"
)

type sdkClient interface {
	CreateSprite(ctx context.Context, name string, labels []string) (*spritesdk.Sprite, error)
	GetSprite(ctx context.Context, name string) (*spritesdk.Sprite, error)
	ListEntries(ctx context.Context, prefix string) ([]SpriteListEntry, error)
	DestroySprite(ctx context.Context, name string) error
	Sprite(name string) *spritesdk.Sprite
	Close() error
}

type realSDKClient struct {
	c *spritesdk.Client
}

func newSDKClient(token string, opts ...spritesdk.Option) *realSDKClient {
	all := append([]spritesdk.Option{}, opts...)
	return &realSDKClient{c: spritesdk.New(token, all...)}
}

func (r *realSDKClient) CreateSprite(ctx context.Context, name string, labels []string) (*spritesdk.Sprite, error) {
	return r.c.CreateSpriteWithOrg(ctx, name, nil, nil, labels)
}

func (r *realSDKClient) GetSprite(ctx context.Context, name string) (*spritesdk.Sprite, error) {
	return r.c.GetSprite(ctx, name)
}

func (r *realSDKClient) DestroySprite(ctx context.Context, name string) error {
	return r.c.DestroySprite(ctx, name)
}

func (r *realSDKClient) Sprite(name string) *spritesdk.Sprite {
	return r.c.Sprite(name)
}

func (r *realSDKClient) Close() error {
	return r.c.Close()
}
