package daytona

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	imagePrefix = "kaxa-"
	// stampLayout is the UTC time in a snapshot name: 16 characters.
	stampLayout = "20060102T150405Z"
)

// Image statuses. Daytona documents no list of them, so anything that is not
// known to be final keeps the wait going until it times out.
var (
	imageReady  = []string{"ready", "available", "completed", "succeeded", "built", "active"}
	imageFailed = []string{statusFailed, statusError}
)

// RestoreMode reports that Daytona restores a snapshot into a new sandbox.
func (p *Provider) RestoreMode() sandbox.RestoreMode { return sandbox.RestoreNewSandbox }

// snapshotName is the image name of a snapshot of the sandbox taken at t. The
// sandbox name is part of it, so a snapshot is traced to its sandbox even after
// the sandbox was destroyed.
func snapshotName(sandboxName string, t time.Time) string {
	return imagePrefix + sandboxName + "-" + t.UTC().Format(stampLayout)
}

// parseSnapshotName returns the sandbox and the time of a snapshot name that
// kaxa made.
func parseSnapshotName(name string) (sandbox.ID, time.Time, bool) {
	rest, ok := strings.CutPrefix(name, imagePrefix)
	if !ok || len(rest) <= len(stampLayout)+1 {
		return "", time.Time{}, false
	}
	cut := len(rest) - len(stampLayout) - 1
	if rest[cut] != '-' {
		return "", time.Time{}, false
	}
	at, err := time.Parse(stampLayout, rest[cut+1:])
	if err != nil || validateName(rest[:cut]) != nil {
		return "", time.Time{}, false
	}
	return sandbox.ID(rest[:cut]), at, true
}

// Snapshot captures the disk of a sandbox as an image and waits until it is
// ready. A stopped sandbox is started first. Memory is not captured.
func (p *Provider) Snapshot(ctx context.Context, id sandbox.ID, comment string) (sandbox.Snapshot, error) {
	if err := sandbox.ValidateSnapshotComment(comment); err != nil {
		return sandbox.Snapshot{}, usererr.Wrap(err, err.Error(), "shorten the comment and remove control characters")
	}
	idx, err := p.index()
	if err != nil {
		return sandbox.Snapshot{}, err
	}
	s, err := p.open(ctx, id)
	if err != nil {
		return sandbox.Snapshot{}, err
	}
	c, _, secrets, err := p.clients()
	if err != nil {
		return sandbox.Snapshot{}, err
	}
	name := snapshotName(string(id), time.Now())
	img, err := c.CaptureImage(ctx, s.rec.ID, name)
	if err != nil {
		return sandbox.Snapshot{}, userErr(err, string(id), secrets...)
	}
	img.Name = name
	if img, err = p.waitImage(ctx, c, img); err != nil {
		return sandbox.Snapshot{}, userErr(err, string(id), secrets...)
	}
	entry := snapshotEntry{Image: name, Comment: strings.TrimSpace(comment), Labels: s.rec.Labels}
	if err := idx.add(entry); err != nil {
		return sandbox.Snapshot{}, usererr.Wrap(err, "kaxa saved the snapshot but could not record its comment",
			"check that the kaxa config directory is writable")
	}
	return toSnapshot(img, id, entry), nil
}

// ListSnapshots returns the snapshots kaxa took of a sandbox, newest first.
// It works after the sandbox was destroyed, because Daytona keeps the image.
func (p *Provider) ListSnapshots(ctx context.Context, id sandbox.ID) ([]sandbox.Snapshot, error) {
	if err := validateName(string(id)); err != nil {
		return nil, err
	}
	all, err := p.ListAllSnapshots(ctx)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(all, func(s sandbox.Snapshot) bool { return s.Sandbox != id }), nil
}

// ListAllSnapshots returns every snapshot kaxa took with this account, newest
// first, including those of sandboxes that were destroyed. Images that kaxa did
// not name are left out.
func (p *Provider) ListAllSnapshots(ctx context.Context) ([]sandbox.Snapshot, error) {
	c, _, secrets, err := p.clients()
	if err != nil {
		return nil, err
	}
	idx, err := p.index()
	if err != nil {
		return nil, err
	}
	images, err := c.ListImages(ctx, imagePrefix)
	if err != nil {
		return nil, userErr(err, "", secrets...)
	}
	entries, err := idx.all()
	if err != nil {
		return nil, usererr.Wrap(err, "kaxa could not read the snapshot comments", "check the kaxa config directory")
	}
	out := make([]sandbox.Snapshot, 0, len(images))
	for _, img := range images {
		owner, at, ok := parseSnapshotName(img.Name)
		if !ok {
			continue
		}
		if img.CreatedAt.IsZero() {
			img.CreatedAt = at
		}
		out = append(out, toSnapshot(img, owner, entries[img.Name]))
	}
	slices.SortStableFunc(out, func(a, b sandbox.Snapshot) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return out, nil
}

// Restore creates req.Target from a snapshot. The source sandbox and the
// snapshot are kept.
func (p *Provider) Restore(ctx context.Context, req sandbox.RestoreRequest) (sandbox.ID, error) {
	if err := validateName(string(req.Target)); err != nil {
		return "", err
	}
	if req.Target == req.Sandbox {
		return "", usererr.New("the target must differ from the source sandbox", "pass a new name")
	}
	c, img, entry, err := p.ownedSnapshot(ctx, req.Sandbox, req.Snapshot)
	if err != nil {
		return "", err
	}
	_, _, secrets, _ := p.clients()
	if !slices.Contains(imageReady, img.Status) {
		return "", usererr.New(
			fmt.Sprintf("snapshot %q is %s and cannot be restored", req.Snapshot, img.Status),
			"wait for the snapshot to be ready, or take a new one")
	}
	if _, err := c.Find(ctx, string(req.Target)); err == nil {
		return "", userErr(ErrAlreadyExists, string(req.Target), secrets...)
	} else if !errors.Is(err, ErrNotFound) {
		return "", userErr(err, string(req.Target), secrets...)
	}
	rec, err := c.Create(ctx, CreateParams{Name: string(req.Target), Labels: entry.Labels, Template: img.Name})
	if err != nil {
		return "", userErr(err, string(req.Target), secrets...)
	}
	if _, err := p.waitRunning(ctx, c, rec); err != nil {
		return "", errors.Join(userErr(err, string(req.Target), secrets...), removeBest(ctx, c, rec.ID))
	}
	return req.Target, nil
}

// DeleteSnapshot deletes a snapshot of the sandbox. A snapshot that is already
// gone counts as deleted.
func (p *Provider) DeleteSnapshot(ctx context.Context, id sandbox.ID, snap sandbox.SnapshotID) error {
	c, img, _, err := p.ownedSnapshot(ctx, id, snap)
	if err != nil {
		return err
	}
	_, _, secrets, _ := p.clients()
	if err := c.DeleteImage(ctx, img.ID); err != nil && !errors.Is(err, ErrImageNotFound) {
		if api, ok := errors.AsType[*APIError](err); ok && api.Status == http.StatusConflict {
			return usererr.Wrap(api, fmt.Sprintf("Daytona cannot delete snapshot %q now", snap),
				"a sandbox restored from it still exists; destroy that sandbox first")
		}
		return userErr(err, string(id), secrets...)
	}
	idx, err := p.index()
	if err != nil {
		return err
	}
	if err := idx.remove(img.Name); err != nil {
		return usererr.Wrap(err, "kaxa deleted the snapshot but could not update its record", "check the kaxa config directory")
	}
	return nil
}

// ownedSnapshot reads an image and checks that kaxa took it of that sandbox.
// Any other image is reported as not found: kaxa never touches the user's own
// images.
func (p *Provider) ownedSnapshot(ctx context.Context, id sandbox.ID, snap sandbox.SnapshotID) (apiClient, Image, snapshotEntry, error) {
	if err := validateName(string(id)); err != nil {
		return nil, Image{}, snapshotEntry{}, err
	}
	if !sandbox.ValidSnapshotID(snap) {
		return nil, Image{}, snapshotEntry{}, usererr.New(fmt.Sprintf("invalid snapshot id %q", snap), "use an id from the snapshot list")
	}
	c, _, secrets, err := p.clients()
	if err != nil {
		return nil, Image{}, snapshotEntry{}, err
	}
	owner, _, parsed := parseSnapshotName(string(snap))
	img, err := c.GetImage(ctx, string(snap))
	if err != nil && !errors.Is(err, ErrImageNotFound) {
		return nil, Image{}, snapshotEntry{}, userErr(err, string(id), secrets...)
	}
	if err != nil || !parsed || owner != id {
		return nil, Image{}, snapshotEntry{}, usererr.New(
			fmt.Sprintf("snapshot %q not found for sandbox %q", snap, id),
			fmt.Sprintf("run kaxa snapshot list %s --provider daytona to see the snapshots", id))
	}
	idx, err := p.index()
	if err != nil {
		return nil, Image{}, snapshotEntry{}, err
	}
	entries, err := idx.all()
	if err != nil {
		return nil, Image{}, snapshotEntry{}, usererr.Wrap(err, "kaxa could not read the snapshot records", "check the kaxa config directory")
	}
	return c, img, entries[img.Name], nil
}

// waitImage polls an image until it is ready. An image that failed is deleted,
// because it holds no usable state.
func (p *Provider) waitImage(ctx context.Context, c apiClient, img Image) (Image, error) {
	timeout, interval := p.polling()
	// Capturing a disk takes longer than booting a sandbox.
	ctx, cancel := context.WithTimeout(ctx, 4*timeout)
	defer cancel()
	for {
		switch {
		case slices.Contains(imageReady, img.Status):
			return img, nil
		case slices.Contains(imageFailed, img.Status):
			return Image{}, errors.Join(imageFailedErr(img), removeImageBest(ctx, c, img))
		}
		if err := sleep(ctx, interval); err != nil {
			return Image{}, fmt.Errorf("daytona: snapshot %q is %s: %w", img.Name, img.Status, err)
		}
		cur, err := c.GetImage(ctx, img.Name)
		if err != nil {
			return Image{}, err
		}
		img = cur
	}
}

func imageFailedErr(img Image) error {
	if img.Error == "" {
		return fmt.Errorf("daytona: snapshot %q failed", img.Name)
	}
	return fmt.Errorf("daytona: snapshot %q failed: %s", img.Name, img.Error)
}

func removeImageBest(ctx context.Context, c apiClient, img Image) error {
	rmCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	if err := c.DeleteImage(rmCtx, img.ID); err != nil && !errors.Is(err, ErrImageNotFound) {
		return fmt.Errorf("remove snapshot: %w", err)
	}
	return nil
}

func toSnapshot(img Image, owner sandbox.ID, e snapshotEntry) sandbox.Snapshot {
	return sandbox.Snapshot{
		ID:        sandbox.SnapshotID(img.Name),
		Sandbox:   owner,
		Provider:  ProviderName,
		Comment:   e.Comment,
		CreatedAt: img.CreatedAt,
	}
}

// SetSnapshotIndexPath sets the file that holds snapshot comments. Tests use it
// to keep the user's config directory out of reach.
func (p *Provider) SetSnapshotIndexPath(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.snapshotIndex = path
}

func (p *Provider) index() (*snapshotIndex, error) {
	p.mu.Lock()
	path := p.snapshotIndex
	p.mu.Unlock()
	if path == "" {
		var err error
		if path, err = defaultIndexPath(); err != nil {
			return nil, usererr.Wrap(err, "kaxa cannot find its config directory", "set XDG_CONFIG_HOME or HOME")
		}
	}
	return newSnapshotIndex(path), nil
}
