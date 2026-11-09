package daytona

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	sdk "github.com/daytona/clients/sdk-go/pkg/daytona"
	sdkerrors "github.com/daytona/clients/sdk-go/pkg/errors"
	"github.com/daytona/clients/sdk-go/pkg/options"
	"github.com/daytona/clients/sdk-go/pkg/types"
)

const (
	defaultAPIURL = "https://app.daytona.io/api"
	pageSize      = 100
	// createTimeout bounds the create call itself. The provider waits for the
	// sandbox to run on its own, with its own timeout.
	createTimeout = 2 * time.Minute
	// snapshotTimeout bounds the capture of a sandbox disk.
	snapshotTimeout = 10 * time.Minute
	deleteTimeout   = 2 * time.Minute

	labelManaged  = "kaxa.managed"
	labelName     = "kaxa.name"
	labelLabels   = "kaxa.labels"
	labelAPIPort  = "kaxa.api_port"
	labelTemplate = "kaxa.template"
	labelTrue     = "true"

	// Statuses kaxa reasons about. Daytona calls a running sandbox "started";
	// the adapter renames it.
	statusRunning = "running"
	statusStopped = "stopped"
	statusPaused  = "paused"
	statusError   = "error"
	statusFailed  = "failed"
)

// CreateParams describes the sandbox to create.
type CreateParams struct {
	Name   string
	Labels []string
	// Template is the Daytona snapshot to start from. Empty means the platform default.
	Template string
	// APIPort is the port of the API that will be published. Zero means none.
	APIPort int
	// NetworkBlockAll blocks every outbound connection from the sandbox.
	NetworkBlockAll bool
}

// Record is one Daytona sandbox as kaxa sees it.
type Record struct {
	// ID is the identifier Daytona generated. Name is the kaxa name.
	ID        string
	Name      string
	Labels    []string
	Status    string
	CreatedAt time.Time
	// APIPort is the port of the published API, or zero.
	APIPort int
	// Template is the snapshot the sandbox was created from. Empty means default.
	Template string

	// Fields below are filled on Get for the list detail view.
	RawState            string
	Snapshot            string
	User                string
	Public              bool
	Target              string
	CPU                 int32
	Memory              int32
	Disk                int32
	GPU                 int32
	GpuType             string
	Spot                bool
	ErrorReason         string
	DesiredState        string
	AutoStopInterval    int
	AutoPauseInterval   int
	AutoArchiveInterval int
	AutoDeleteInterval  int
	AutoDestroyAt       string
	LastActivityAt      string
	UpdatedAt           string
	SandboxClass        string
	VolumeMounts        []string
	Kvm                 bool
	NetworkBlockAll     bool
	NetworkAllowList    string
	DomainAllowList     string
}

// Image is a disk snapshot captured from a sandbox.
type Image struct {
	ID        string
	Name      string
	Status    string
	Error     string
	CreatedAt time.Time
}

// PortEndpoint is the preview address of a sandbox port.
type PortEndpoint struct {
	Port   int
	Public bool
	URL    string
}

type apiClient interface {
	Create(ctx context.Context, p CreateParams) (Record, error)
	// List returns the sandboxes kaxa manages, except deleted ones.
	List(ctx context.Context) ([]Record, error)
	// Find returns the managed sandbox called name, or ErrNotFound.
	Find(ctx context.Context, name string) (Record, error)
	Get(ctx context.Context, id string) (Record, error)
	Delete(ctx context.Context, id string) error
	Start(ctx context.Context, id string) error
	Resume(ctx context.Context, id string) error
	Stop(ctx context.Context, id string) error
	// SetPort makes the sandbox public when asked and returns the preview of port.
	// Daytona makes every port of a sandbox public together, not one by one.
	SetPort(ctx context.Context, id string, port int, public bool) (PortEndpoint, error)
	GetPort(ctx context.Context, id string, port int) (PortEndpoint, error)
	CaptureImage(ctx context.Context, id, name string) (Image, error)
	// ListImages returns the named images whose name starts with prefix.
	ListImages(ctx context.Context, prefix string) ([]Image, error)
	// ListSnapshotCatalog returns every snapshot the account can see, including
	// platform snapshots and ones that are not active. The caller filters.
	ListSnapshotCatalog(ctx context.Context) ([]snapshotCatalogRow, error)
	GetImage(ctx context.Context, name string) (Image, error)
	DeleteImage(ctx context.Context, id string) error
}

// sdkClient is the apiClient over the official Daytona Go SDK.
type sdkClient struct {
	c *sdk.Client
	// handles keeps the last sandbox read per ID. The SDK handle only holds
	// the sandbox ID and its toolbox address, which never change, so process
	// calls reuse it instead of reading the sandbox again.
	handles sync.Map
}

func newSDKClient(apiKey string) (*sdkClient, error) {
	// Polling is deprecated, but the event stream of the SDK can stall behind
	// proxies, and kaxa waits on its own timeouts.
	polling := true
	c, err := sdk.NewClientWithConfig(&types.DaytonaConfig{
		APIKey: apiKey, APIUrl: defaultAPIURL,
		UseDeprecatedPolling: &polling, //nolint:staticcheck // see the comment above
	})
	if err != nil {
		return nil, convertErr(err)
	}
	return &sdkClient{c: c}, nil
}

// convertErr turns an SDK error into an *APIError, which the provider knows
// how to explain to the user. Other errors pass through.
func convertErr(err error) error {
	if err == nil {
		return nil
	}
	if de, ok := errors.AsType[*sdkerrors.DaytonaError](err); ok && de.StatusCode != 0 {
		return &APIError{Status: de.StatusCode, Message: de.Message}
	}
	return err
}

func (s *sdkClient) get(ctx context.Context, id string) (*sdk.Sandbox, error) {
	sb, err := s.c.Get(ctx, id)
	if err != nil {
		return nil, notFound(convertErr(err))
	}
	s.handles.Store(sb.ID, sb)
	// The name is stable and is how kaxa addresses the sandbox. Caching it too
	// lets the next command reuse this handle instead of reading the sandbox
	// again, which can miss a sandbox whose labels are not visible yet.
	if sb.Name != "" {
		s.handles.Store(sb.Name, sb)
	}
	return sb, nil
}

// handle returns the cached sandbox for id, or reads it.
func (s *sdkClient) handle(ctx context.Context, id string) (*sdk.Sandbox, error) {
	if sb, ok := s.handles.Load(id); ok {
		if cached, ok := sb.(*sdk.Sandbox); ok {
			return cached, nil
		}
	}
	return s.get(ctx, id)
}

func (s *sdkClient) Create(ctx context.Context, p CreateParams) (Record, error) {
	labels := map[string]string{
		labelManaged: labelTrue,
		labelName:    p.Name,
		labelLabels:  strings.Join(p.Labels, ","),
	}
	if p.APIPort > 0 {
		labels[labelAPIPort] = strconv.Itoa(p.APIPort)
	}
	if p.Template != "" {
		labels[labelTemplate] = p.Template
	}
	sb, err := s.c.Create(ctx, types.SnapshotParams{
		Snapshot:        p.Template,
		Name:            p.Name,
		Labels:          labels,
		NetworkBlockAll: p.NetworkBlockAll,
	}, options.WithTimeout(createTimeout), options.WithWaitForStart(false))
	if err != nil {
		return Record{}, conflict(convertErr(err))
	}
	rec, ok := recordOf(sb)
	if !ok {
		return Record{}, errors.New("daytona: create returned no managed sandbox")
	}
	return rec, nil
}

// conflict maps the answer for a name that is taken.
func conflict(err error) error {
	if api, ok := errors.AsType[*APIError](err); ok && api.Status == 409 {
		return ErrAlreadyExists
	}
	return err
}

// List returns every managed sandbox. It reads the sandboxes by label, so it
// never shows one that kaxa did not create.
func (s *sdkClient) List(ctx context.Context) ([]Record, error) {
	limit := pageSize
	it := s.c.List(ctx, &sdk.ListSandboxesQuery{Limit: &limit, Labels: map[string]string{labelManaged: labelTrue}})
	var records []Record
	for it.Next() {
		if rec, ok := recordOf(it.Value()); ok {
			records = append(records, rec)
		}
	}
	if err := it.Err(); err != nil {
		return nil, convertErr(err)
	}
	return records, nil
}

// Find reads the sandbox by name. Daytona keeps names unique, and reading by
// name sees a new sandbox at once, which the label listing does not.
func (s *sdkClient) Find(ctx context.Context, name string) (Record, error) {
	rec, err := s.Get(ctx, name)
	if err != nil {
		return Record{}, err
	}
	if rec.Name != name {
		return Record{}, ErrNotFound
	}
	return rec, nil
}

func (s *sdkClient) Get(ctx context.Context, id string) (Record, error) {
	sb, err := s.get(ctx, id)
	if err != nil {
		return Record{}, err
	}
	rec, ok := recordOf(sb)
	if !ok {
		return Record{}, ErrNotFound
	}
	enrichRecord(&rec, sb)
	return rec, nil
}

// statusOf renames a Daytona sandbox state to the status kaxa uses.
func statusOf(state string) string {
	switch state {
	case "started":
		return statusRunning
	case "build_failed":
		return statusError
	case "destroying", "destroyed":
		return "deleted"
	}
	return state
}

// recordOf converts a sandbox. A sandbox that kaxa did not create, or that is
// being deleted, is not a record.
func recordOf(sb *sdk.Sandbox) (Record, bool) {
	if sb == nil || sb.Labels[labelManaged] != labelTrue || sb.Labels[labelName] == "" {
		return Record{}, false
	}
	status := statusOf(string(sb.State))
	if status == "deleted" {
		return Record{}, false
	}
	rec := Record{ID: sb.ID, Name: sb.Labels[labelName], Status: status, Template: sb.Labels[labelTemplate]}
	if sb.CreatedAt != nil {
		rec.CreatedAt, _ = time.Parse(time.RFC3339, *sb.CreatedAt)
	}
	if raw := sb.Labels[labelLabels]; raw != "" {
		rec.Labels = slices.DeleteFunc(strings.Split(raw, ","), func(l string) bool { return l == "" })
	}
	if port, err := strconv.Atoi(sb.Labels[labelAPIPort]); err == nil && port > 0 {
		rec.APIPort = port
	}
	return rec, true
}

func (s *sdkClient) Delete(ctx context.Context, id string) error {
	sb, err := s.get(ctx, id)
	if err != nil {
		return err
	}
	s.handles.Delete(sb.ID)
	s.handles.Delete(sb.Name)
	return notFound(convertErr(sb.DeleteAndWait(ctx, deleteTimeout)))
}

func (s *sdkClient) Start(ctx context.Context, id string) error {
	sb, err := s.get(ctx, id)
	if err != nil {
		return err
	}
	return notFound(convertErr(sb.Start(ctx)))
}

// Resume wakes a paused sandbox. Daytona starts a paused sandbox the same way
// as a stopped one.
func (s *sdkClient) Resume(ctx context.Context, id string) error {
	return s.Start(ctx, id)
}

func (s *sdkClient) Stop(ctx context.Context, id string) error {
	sb, err := s.get(ctx, id)
	if err != nil {
		return err
	}
	return notFound(convertErr(sb.Stop(ctx)))
}

func (s *sdkClient) SetPort(ctx context.Context, id string, port int, public bool) (PortEndpoint, error) {
	sb, err := s.get(ctx, id)
	if err != nil {
		return PortEndpoint{}, err
	}
	if public {
		if err := sb.SetPublic(ctx, true); err != nil {
			return PortEndpoint{}, convertErr(err)
		}
	}
	return s.preview(ctx, sb, port)
}

func (s *sdkClient) GetPort(ctx context.Context, id string, port int) (PortEndpoint, error) {
	sb, err := s.get(ctx, id)
	if err != nil {
		return PortEndpoint{}, err
	}
	return s.preview(ctx, sb, port)
}

func (s *sdkClient) preview(ctx context.Context, sb *sdk.Sandbox, port int) (PortEndpoint, error) {
	link, err := sb.GetPreviewLink(ctx, port)
	if err != nil {
		return PortEndpoint{}, convertErr(err)
	}
	return PortEndpoint{Port: port, Public: sb.Public, URL: link.URL}, nil
}

// CaptureImage snapshots the disk of the sandbox and returns once Daytona has
// finished, so the result is the snapshot as the API reports it.
func (s *sdkClient) CaptureImage(ctx context.Context, id, name string) (Image, error) {
	sb, err := s.get(ctx, id)
	if err != nil {
		return Image{}, err
	}
	if err := sb.CreateSnapshotWithTimeout(ctx, name, snapshotTimeout); err != nil {
		return Image{}, convertErr(err)
	}
	return s.GetImage(ctx, name)
}

// snapshotCatalogRow is one snapshot from Snapshot.List, before filtering.
type snapshotCatalogRow struct {
	Name    string
	CPU     int
	Memory  int
	Disk    int
	General bool
	State   string
}

const snapshotStateActive = "active"

func (s *sdkClient) ListSnapshotCatalog(ctx context.Context) ([]snapshotCatalogRow, error) {
	var rows []snapshotCatalogRow
	limit := pageSize
	for page := 1; ; page++ {
		got, err := s.c.Snapshot.List(ctx, &page, &limit)
		if err != nil {
			return nil, convertErr(err)
		}
		for _, snap := range got.Items {
			if snap == nil {
				continue
			}
			rows = append(rows, snapshotCatalogRow{
				Name: snap.Name, CPU: snap.CPU, Memory: snap.Memory, Disk: snap.Disk,
				General: snap.General, State: snap.State,
			})
		}
		if page >= got.TotalPages || len(got.Items) == 0 {
			return rows, nil
		}
	}
}

func (s *sdkClient) ListImages(ctx context.Context, prefix string) ([]Image, error) {
	var images []Image
	limit := pageSize
	for page := 1; ; page++ {
		got, err := s.c.Snapshot.List(ctx, &page, &limit)
		if err != nil {
			return nil, convertErr(err)
		}
		for _, snap := range got.Items {
			if strings.HasPrefix(snap.Name, prefix) {
				images = append(images, imageOf(snap))
			}
		}
		if page >= got.TotalPages || len(got.Items) == 0 {
			return images, nil
		}
	}
}

func (s *sdkClient) GetImage(ctx context.Context, name string) (Image, error) {
	snap, err := s.c.Snapshot.Get(ctx, name)
	if err != nil {
		return Image{}, imageNotFound(convertErr(err))
	}
	return imageOf(snap), nil
}

func (s *sdkClient) DeleteImage(ctx context.Context, id string) error {
	return imageNotFound(convertErr(s.c.Snapshot.DeleteByNameOrID(ctx, id)))
}

func imageNotFound(err error) error {
	if api, ok := errors.AsType[*APIError](err); ok && api.Status == 404 {
		return ErrImageNotFound
	}
	return err
}

// imageOf converts a snapshot. Daytona calls a usable snapshot "active"; the
// states that never recover are reported as "failed".
func imageOf(snap *types.Snapshot) Image {
	img := Image{ID: snap.ID, Name: snap.Name, Status: snap.State, CreatedAt: snap.CreatedAt}
	switch snap.State {
	case statusError, "build_failed":
		img.Status = statusFailed
	}
	if snap.ErrorReason != nil {
		img.Error = *snap.ErrorReason
	}
	return img
}
