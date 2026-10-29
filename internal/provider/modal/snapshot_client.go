package modal

import (
	"context"
	"errors"
	"time"

	sdk "github.com/modal-labs/modal-client/go"
)

// snapshotTimeout is how long a filesystem snapshot may take.
const snapshotTimeout = 5 * time.Minute

func (r *realSDKClient) SnapshotFilesystem(ctx context.Context, name string) (string, error) {
	sb, err := r.c.Sandboxes.FromName(ctx, appName, name, nil)
	if err != nil {
		return "", notFound(err)
	}
	defer func() { _ = sb.Detach() }()
	// The SDK default is 55 s, too short for a large filesystem. The TTL stays
	// zero, which keeps the image for 30 days.
	img, err := sb.SnapshotFilesystem(ctx, &sdk.SandboxSnapshotFilesystemParams{Timeout: snapshotTimeout})
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
