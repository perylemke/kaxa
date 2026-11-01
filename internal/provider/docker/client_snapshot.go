package docker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Snapshot statuses of the API.
const (
	snapshotReady    = "ready"
	snapshotCreating = "creating"
	snapshotFailed   = "failed"
	snapshotDeleted  = "deleted"
)

const (
	// snapshotPrefix and snapshotStamp build the display name of a kaxa
	// snapshot: kaxa-<sandbox>-<yyyymmddhhmmss>. The list view of the API has
	// no description or labels, so the name is what tells a snapshot of kaxa,
	// and its sandbox, from the user's own snapshots.
	snapshotPrefix = "kaxa-"
	snapshotStamp  = "20060102150405"
	// captureDisk saves the disk only. Memory capture needs support that not
	// every sandbox has, and a restore starts the sandbox again anyway.
	captureDisk = "disk"
)

// SnapshotRecord is a snapshot as kaxa sees it.
type SnapshotRecord struct {
	// ID is the immutable ID, the last segment of Resource.
	ID string
	// Resource is the API resource name, such as snapshots/abc.
	Resource    string
	DisplayName string
	// Description holds the comment. The list view of the API leaves it empty.
	Description string
	Status      string
	Failure     string
	Etag        string
	CreatedAt   time.Time
}

// Sandbox returns the name of the kaxa sandbox the snapshot was taken from.
// It is empty for a snapshot that kaxa did not take.
func (s SnapshotRecord) Sandbox() string {
	name, ok := snapshotSandbox(s.DisplayName)
	if !ok {
		return ""
	}
	return name
}

// snapshotMeta is what kaxa stores in the description of a snapshot: the
// comment, and the labels of the sandbox, which a restored sandbox needs
// because Docker gives it none.
type snapshotMeta struct {
	Labels  []string `json:"labels,omitempty"`
	Comment string   `json:"comment,omitempty"`
}

// encodeSnapshotMeta returns the description for a snapshot, empty when there
// is nothing to store.
func encodeSnapshotMeta(labels []string, comment string) string {
	if len(labels) == 0 && comment == "" {
		return ""
	}
	raw, err := json.Marshal(snapshotMeta{Labels: labels, Comment: comment})
	if err != nil {
		// Strings cannot fail to encode; keep the comment alone if it ever does.
		return comment
	}
	return string(raw)
}

// meta reads the description of the snapshot. A description that kaxa did not
// write is taken as the comment.
func (s SnapshotRecord) meta() snapshotMeta {
	var m snapshotMeta
	if strings.HasPrefix(s.Description, "{") && json.Unmarshal([]byte(s.Description), &m) == nil {
		return m
	}
	return snapshotMeta{Comment: s.Description}
}

// snapshotDisplayName is the display name kaxa gives a snapshot of a sandbox.
func snapshotDisplayName(sandboxName string, at time.Time) string {
	return snapshotPrefix + sandboxName + "-" + at.UTC().Format(snapshotStamp)
}

// snapshotSandbox reads the sandbox name out of a display name made by
// snapshotDisplayName.
func snapshotSandbox(displayName string) (string, bool) {
	rest, ok := strings.CutPrefix(displayName, snapshotPrefix)
	if !ok {
		return "", false
	}
	name, stamp, ok := strings.CutLast(rest, "-")
	if !ok || validateName(name) != nil {
		return "", false
	}
	if _, err := time.Parse(snapshotStamp, stamp); err != nil {
		return "", false
	}
	return name, true
}

// snapshotAPI is the snapshot part of the management API.
type snapshotAPI interface {
	// CreateSnapshot saves the disk of a running sandbox. The snapshot may
	// still be creating when it returns.
	CreateSnapshot(ctx context.Context, r Record, displayName, description string) (SnapshotRecord, error)
	// ListSnapshots returns every snapshot of the account.
	ListSnapshots(ctx context.Context) ([]SnapshotRecord, error)
	GetSnapshot(ctx context.Context, resource string) (SnapshotRecord, error)
	DeleteSnapshot(ctx context.Context, s SnapshotRecord) error
	// RestoreSnapshot forks a new sandbox named name from a snapshot.
	RestoreSnapshot(ctx context.Context, s SnapshotRecord, name string) (Record, error)
}

// wireSnapshot is the part of the Snapshot resource kaxa reads. The list view
// uses the same fields, without description and failure.
type wireSnapshot struct {
	UID         string     `json:"uid"`
	Name        string     `json:"name"`
	DisplayName string     `json:"displayName"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	CreatedAt   string     `json:"createdAt"`
	Etag        string     `json:"etag"`
	Failure     *wireError `json:"failure"`
}

func (w wireSnapshot) record(headerEtag string) SnapshotRecord {
	s := SnapshotRecord{
		ID:          w.UID,
		Resource:    w.Name,
		DisplayName: w.DisplayName,
		Description: w.Description,
		Status:      w.Status,
		Etag:        w.Etag,
	}
	if s.ID == "" {
		s.ID = resourceID(w.Name)
	}
	if headerEtag != "" {
		s.Etag = headerEtag
	}
	if w.Failure != nil {
		s.Failure = w.Failure.Message
		if s.Failure == "" {
			s.Failure = w.Failure.Code
		}
	}
	if t, err := time.Parse(time.RFC3339Nano, w.CreatedAt); err == nil {
		s.CreatedAt = t
	}
	return s
}

type snapshotListResponse struct {
	Snapshots     []wireSnapshot `json:"snapshots"`
	NextPageToken string         `json:"nextPageToken"`
}

type snapshotBody struct {
	DisplayName string `json:"displayName"`
	Description string `json:"description,omitempty"`
	CaptureMode string `json:"captureMode"`
}

// CreateSnapshot saves the disk of a running sandbox. The idempotency key lets
// a retry after a lost response return the first snapshot instead of a second.
func (c *restClient) CreateSnapshot(ctx context.Context, r Record, displayName, description string) (SnapshotRecord, error) {
	key, err := idempotencyKey()
	if err != nil {
		return SnapshotRecord{}, err
	}
	var w wireSnapshot
	var etag string
	err = c.call(ctx, request{
		method:  http.MethodPost,
		url:     c.base + "/v1/" + r.Resource + "/snapshots",
		headers: map[string]string{idempotencyHeader: key},
		in:      snapshotBody{DisplayName: displayName, Description: description, CaptureMode: captureDisk},
		out:     &w,
		etag:    &etag,
	})
	if apiErr, ok := errors.AsType[*APIError](err); ok && apiErr.is(codeAlreadyExists, http.StatusConflict) {
		return SnapshotRecord{}, ErrSnapshotExists
	}
	if err != nil {
		return SnapshotRecord{}, err
	}
	return w.record(etag), nil
}

// ListSnapshots returns every snapshot of the account, page by page.
func (c *restClient) ListSnapshots(ctx context.Context) ([]SnapshotRecord, error) {
	var snaps []SnapshotRecord
	token := ""
	for range maxPages {
		q := url.Values{"pageSize": {pageSize}}
		if token != "" {
			q.Set("pageToken", token)
		}
		var out snapshotListResponse
		if err := c.call(ctx, request{method: http.MethodGet, url: c.base + "/v1/snapshots", query: q, out: &out}); err != nil {
			return nil, err
		}
		for _, w := range out.Snapshots {
			snaps = append(snaps, w.record(""))
		}
		if out.NextPageToken == "" {
			return snaps, nil
		}
		token = out.NextPageToken
	}
	return snaps, nil
}

// GetSnapshot reads one snapshot by resource name.
func (c *restClient) GetSnapshot(ctx context.Context, resource string) (SnapshotRecord, error) {
	var w wireSnapshot
	var etag string
	err := c.call(ctx, request{method: http.MethodGet, url: c.base + "/v1/" + resource, out: &w, etag: &etag})
	if isNotFound(err) {
		return SnapshotRecord{}, ErrSnapshotNotFound
	}
	if err != nil {
		return SnapshotRecord{}, err
	}
	return w.record(etag), nil
}

// DeleteSnapshot deletes a snapshot. Docker answers success for one that is
// already gone, so ErrSnapshotNotFound is rare.
func (c *restClient) DeleteSnapshot(ctx context.Context, s SnapshotRecord) error {
	err := c.call(ctx, request{
		method:  http.MethodDelete,
		url:     c.base + "/v1/" + s.Resource,
		headers: map[string]string{"If-Match": s.Etag},
	})
	if isNotFound(err) {
		return ErrSnapshotNotFound
	}
	return err
}

type restoreBody struct {
	DisplayName string `json:"displayName"`
}

// RestoreSnapshot forks a new sandbox from a snapshot. The source sandbox and
// the snapshot are kept.
func (c *restClient) RestoreSnapshot(ctx context.Context, s SnapshotRecord, name string) (Record, error) {
	key, err := idempotencyKey()
	if err != nil {
		return Record{}, err
	}
	var w wireSandbox
	var etag string
	err = c.call(ctx, request{
		method:  http.MethodPost,
		url:     c.base + "/v1/" + s.Resource + "/restore",
		headers: map[string]string{idempotencyHeader: key},
		in:      restoreBody{DisplayName: name},
		out:     &w,
		etag:    &etag,
	})
	if apiErr, ok := errors.AsType[*APIError](err); ok {
		switch {
		case apiErr.is(codeAlreadyExists, http.StatusConflict):
			return Record{}, ErrAlreadyExists
		case apiErr.is(codeNotFound, http.StatusNotFound):
			return Record{}, ErrSnapshotNotFound
		}
	}
	if err != nil {
		return Record{}, err
	}
	return w.record(etag), nil
}
