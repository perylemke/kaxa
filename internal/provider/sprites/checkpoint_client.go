package sprites

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	spritesdk "github.com/superfly/sprites-go"
)

// checkpointClient is the checkpoint part of the Sprites API. It is separate
// from sdkClient so a client without checkpoints still serves every other command.
type checkpointClient interface {
	CreateCheckpoint(ctx context.Context, name, comment string) error
	ListCheckpoints(ctx context.Context, name string) ([]*spritesdk.Checkpoint, error)
	RestoreCheckpoint(ctx context.Context, name, id string) error
	DeleteCheckpoint(ctx context.Context, name, id string) error
}

func (r *realSDKClient) CreateCheckpoint(ctx context.Context, name, comment string) error {
	stream, err := r.c.CreateCheckpointWithComment(ctx, name, comment)
	if err != nil {
		return err
	}
	defer func() { _ = stream.Close() }()
	return drainStream(stream.Next)
}

func (r *realSDKClient) ListCheckpoints(ctx context.Context, name string) ([]*spritesdk.Checkpoint, error) {
	return r.c.ListCheckpoints(ctx, name, "")
}

func (r *realSDKClient) RestoreCheckpoint(ctx context.Context, name, id string) error {
	stream, err := r.c.RestoreCheckpoint(ctx, name, id)
	if err != nil {
		return err
	}
	defer func() { _ = stream.Close() }()
	return drainStream(stream.Next)
}

// DeleteCheckpoint calls the checkpoint delete route. The route is not in the
// public API reference, so its failures say what was asked.
func (r *realSDKClient) DeleteCheckpoint(ctx context.Context, name, id string) error {
	path := fmt.Sprintf("/v1/sprites/%s/checkpoints/%s", url.PathEscape(name), url.PathEscape(id))
	return spriteRequest(ctx, http.MethodDelete, r.baseURL, r.token, path)
}

// drainStream reads a progress stream to its end and fails on an error message.
func drainStream(next func() (*spritesdk.StreamMessage, error)) error {
	for {
		msg, err := next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if msg.Type == "error" {
			text := msg.Error
			if text == "" {
				text = msg.Data
			}
			return fmt.Errorf("sprites: %s", text)
		}
	}
}
