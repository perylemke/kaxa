package blaxel

import (
	"context"
	"errors"
	"net/url"

	sdk "github.com/blaxel-ai/sdk-go"
)

func (s *sdkClient) CreateSnapshot(ctx context.Context, sandboxName string) (snapshotRow, error) {
	got, err := s.snapshots.New(ctx, sdk.SnapshotNewParams{
		SandboxSnapshotRequest: sdk.SandboxSnapshotRequestParam{
			Source: sdk.SandboxSnapshotSourceParam{
				Name: sandboxName,
				Kind: sdk.SandboxSnapshotSourceKindSandbox,
			},
		},
	})
	if err != nil {
		return snapshotRow{}, convertErr(err)
	}
	return snapshotFrom(got)
}

func (s *sdkClient) GetSnapshot(ctx context.Context, id string) (snapshotRow, error) {
	got, err := s.snapshots.Get(ctx, id)
	if err != nil {
		return snapshotRow{}, convertErr(err)
	}
	return snapshotFrom(got)
}

func (s *sdkClient) ListSandboxSnapshots(ctx context.Context, sandboxName string) ([]snapshotRow, error) {
	got, err := s.sandboxSnapshots.List(ctx, sandboxName)
	if err != nil {
		return nil, convertErr(err)
	}
	return snapshotRows(got), nil
}

func (s *sdkClient) ListWorkspaceSnapshots(ctx context.Context) ([]snapshotRow, error) {
	page, err := s.snapshots.List(ctx, sdk.SnapshotListParams{Limit: sdk.Int(200)})
	if err != nil {
		return nil, convertErr(err)
	}
	var out []snapshotRow
	for page != nil {
		out = append(out, snapshotRows(&page.Data)...)
		page, err = page.GetNextPage()
		if err != nil {
			return nil, convertErr(err)
		}
	}
	return out, nil
}

func (s *sdkClient) DeleteSnapshot(ctx context.Context, id string) error {
	return convertErr(s.snapshots.Delete(ctx, id))
}

func (s *sdkClient) RestoreSnapshot(ctx context.Context, sandboxName, id string) error {
	_, err := s.sandboxSnapshots.Restore(ctx, id, sdk.SandboxSnapshotRestoreParams{SandboxName: sandboxName})
	return convertErr(err)
}

func (s *sdkClient) Unarchive(ctx context.Context, name string) (Record, error) {
	var sb sdk.Sandbox
	client := sdk.Client{Options: s.opts}
	path := "sandboxes/" + url.PathEscape(name) + "/unarchive"
	if err := client.Post(ctx, path, nil, &sb); err != nil {
		return Record{}, convertErr(err)
	}
	rec, ok := recordFrom(&sb)
	if !ok {
		return Record{}, errors.New("blaxel: unarchive returned an unmanaged sandbox")
	}
	return rec, nil
}

func snapshotFrom(got *sdk.SandboxSnapshot) (snapshotRow, error) {
	if got == nil || got.ID == "" {
		return snapshotRow{}, errors.New("blaxel: snapshot has no id")
	}
	owner := got.Source.Name
	if owner == "" {
		owner = got.SandboxName
	}
	return snapshotRow{
		ID: got.ID, Name: got.Name, Sandbox: owner, Status: got.Status, CreatedAt: parseTime(got.CreatedAt),
	}, nil
}

func snapshotRows(got *[]sdk.SandboxSnapshot) []snapshotRow {
	if got == nil {
		return nil
	}
	out := make([]snapshotRow, 0, len(*got))
	for i := range *got {
		row, err := snapshotFrom(&(*got)[i])
		if err != nil {
			continue
		}
		out = append(out, row)
	}
	return out
}
