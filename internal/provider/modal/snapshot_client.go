package modal

import (
	"context"
	"errors"

	sdk "github.com/modal-labs/modal-client/go"
)

func (r *realSDKClient) SnapshotFilesystem(ctx context.Context, name string) (string, error) {
	sb, err := r.c.Sandboxes.FromName(ctx, appName, name, nil)
	if err != nil {
		return "", notFound(err)
	}
	defer func() { _ = sb.Detach() }()
	// nil params: Modal keeps the image for 30 days.
	img, err := sb.SnapshotFilesystem(ctx, nil)
	if err != nil {
		return "", err
	}
	return img.ImageID, nil
}

func (r *realSDKClient) CreateFromImage(ctx context.Context, name string, labels []string, imageID string) (Record, error) {
	img, err := r.c.Images.FromID(ctx, imageID, nil)
	if err != nil {
		return Record{}, imageNotFound(err)
	}
	return r.create(ctx, name, labels, img)
}

func (r *realSDKClient) DeleteImage(ctx context.Context, imageID string) error {
	return imageNotFound(r.c.Images.Delete(ctx, imageID, nil))
}

func imageNotFound(err error) error {
	if _, missing := errors.AsType[sdk.NotFoundError](err); missing {
		return ErrImageNotFound
	}
	return err
}
