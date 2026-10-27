package sprites

import (
	"context"
	"errors"
	"fmt"
	"io"

	spritesdk "github.com/superfly/sprites-go"
)

type sdkClient interface {
	CreateSprite(ctx context.Context, name string, labels []string) (*spritesdk.Sprite, error)
	GetSprite(ctx context.Context, name string) (*spritesdk.Sprite, error)
	ListEntries(ctx context.Context, prefix string) ([]SpriteListEntry, error)
	UpgradeSprite(ctx context.Context, name string) error
	RestartSprite(ctx context.Context, name string) error
	Run(ctx context.Context, name string, argv []string, stdin io.Reader, stdout, stderr io.Writer) error
	RunIn(ctx context.Context, name, dir string, env, argv []string, stdin io.Reader, stdout, stderr io.Writer) error
	DestroySprite(ctx context.Context, name string) error
	Sprite(name string) *spritesdk.Sprite
	Close() error
}

type realSDKClient struct {
	c       *spritesdk.Client
	token   string
	baseURL string
}

func newSDKClient(token, baseURL string, opts ...spritesdk.Option) *realSDKClient {
	if baseURL == "" {
		baseURL = defaultAPIBase
	}
	all := make([]spritesdk.Option, 0, 1+len(opts))
	all = append(all, spritesdk.WithBaseURL(baseURL))
	for _, opt := range opts {
		if opt != nil {
			all = append(all, opt)
		}
	}
	return &realSDKClient{
		c:       spritesdk.New(token, all...),
		token:   token,
		baseURL: baseURL,
	}
}

func (r *realSDKClient) CreateSprite(ctx context.Context, name string, labels []string) (*spritesdk.Sprite, error) {
	return r.c.CreateSpriteWithOrg(ctx, name, nil, nil, labels)
}

func (r *realSDKClient) GetSprite(ctx context.Context, name string) (*spritesdk.Sprite, error) {
	return r.c.GetSprite(ctx, name)
}

func (r *realSDKClient) UpgradeSprite(ctx context.Context, name string) error {
	return r.c.UpgradeSprite(ctx, name)
}

func (r *realSDKClient) RestartSprite(ctx context.Context, name string) error {
	return postSpriteAction(ctx, r.baseURL, r.token, fmt.Sprintf("/v1/sprites/%s/restart", name))
}

func (r *realSDKClient) Run(ctx context.Context, name string, argv []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return r.RunIn(ctx, name, "", nil, argv, stdin, stdout, stderr)
}

func (r *realSDKClient) RunIn(ctx context.Context, name, dir string, env, argv []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(argv) == 0 {
		return errors.New("sprites run: empty command")
	}
	cmd := r.c.Sprite(name).CommandContext(ctx, argv[0], argv[1:]...)
	if dir != "" {
		cmd.Dir = dir
	}
	if len(env) > 0 {
		cmd.Env = env
	}
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
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
