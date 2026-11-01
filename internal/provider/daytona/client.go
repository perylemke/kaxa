package daytona

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAPIURL     = "https://api.daytona.com"
	defaultToolboxURL = "https://toolbox.daytona.com"
	maxPageSize       = 100
	maxErrorBody      = 4 << 10

	labelManaged  = "kaxa.managed"
	labelName     = "kaxa.name"
	labelLabels   = "kaxa.labels"
	labelAPIPort  = "kaxa.api_port"
	labelTemplate = "kaxa.template"
	labelTrue     = "true"

	statusRunning = "running"
	statusStopped = "stopped"
	statusPaused  = "paused"
)

// CreateParams describes the sandbox to create.
type CreateParams struct {
	Name   string
	Labels []string
	// Template is the image to start from. Empty means the platform default.
	Template string
	// APIPort is the port of the API that will be published. Zero means none.
	APIPort int
}

// Record is one Daytona sandbox as kaxa sees it.
type Record struct {
	// ID is the identifier Daytona generated. Name is the kaxa name.
	ID         string
	Name       string
	Labels     []string
	Status     string
	CreatedAt  time.Time
	ToolboxURL string
	// APIPort is the port of the published API, or zero.
	APIPort int
	// Template is the image the sandbox was created from. Empty means default.
	Template string
}

// Image is a disk image captured from a sandbox.
type Image struct {
	ID        string
	Name      string
	Status    string
	Error     string
	CreatedAt time.Time
}

// PortEndpoint is an exposed sandbox port.
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
	SetPort(ctx context.Context, id string, port int, public bool) (PortEndpoint, error)
	GetPort(ctx context.Context, id string, port int) (PortEndpoint, error)
	CaptureImage(ctx context.Context, id, name string) (Image, error)
	// ListImages returns the named images whose name starts with prefix.
	ListImages(ctx context.Context, prefix string) ([]Image, error)
	GetImage(ctx context.Context, name string) (Image, error)
	DeleteImage(ctx context.Context, id string) error
}

type restClient struct {
	base *url.URL
	key  string
	http *http.Client
}

func newRESTClient(apiKey string) *restClient {
	c, _ := newRESTClientAt(defaultAPIURL, apiKey)
	return c
}

func newRESTClientAt(base, apiKey string) (*restClient, error) {
	u, err := checkEndpoint(base)
	if err != nil {
		return nil, err
	}
	return &restClient{base: u, key: apiKey, http: &http.Client{Timeout: 2 * time.Minute}}, nil
}

type sandboxJSON struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Status     string            `json:"status"`
	Labels     map[string]string `json:"labels"`
	CreatedAt  time.Time         `json:"created_at"`
	ToolboxURL string            `json:"toolbox_proxy_url"`
}

type sandboxPage struct {
	Items      []sandboxJSON `json:"items"`
	NextCursor string        `json:"next_cursor"`
}

type imageJSON struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Error     string    `json:"error"`
	CreatedAt time.Time `json:"created_at"`
}

type imagePage struct {
	Items      []imageJSON `json:"items"`
	NextCursor string      `json:"next_cursor"`
}

type portJSON struct {
	Port   int    `json:"port"`
	Public bool   `json:"public"`
	URL    string `json:"url"`
}

func (c *restClient) Create(ctx context.Context, p CreateParams) (Record, error) {
	labels := map[string]string{
		labelManaged: labelTrue,
		labelName:    p.Name,
		labelLabels:  strings.Join(p.Labels, ","),
	}
	if p.APIPort > 0 {
		labels[labelAPIPort] = strconv.Itoa(p.APIPort)
	}
	body := map[string]any{"name": p.Name, "persistent": true, "labels": labels}
	if p.Template != "" {
		labels[labelTemplate] = p.Template
		body["image"] = p.Template
	}
	var out sandboxJSON
	if err := c.do(ctx, http.MethodPost, "/sandboxes", nil, body, &out); err != nil {
		return Record{}, err
	}
	rec, ok := recordOf(out)
	if !ok || rec.ID == "" {
		return Record{}, errors.New("daytona: create returned no sandbox ID")
	}
	return rec, nil
}

// List pages through every sandbox of the project and keeps the managed ones.
// It reads the control plane directly: the search index is eventually
// consistent, so a sandbox created a moment ago could be missing from it.
func (c *restClient) List(ctx context.Context) ([]Record, error) {
	var records []Record
	cursor := ""
	for {
		q := url.Values{"limit": {strconv.Itoa(maxPageSize)}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var page sandboxPage
		if err := c.do(ctx, http.MethodGet, "/sandboxes", q, nil, &page); err != nil {
			return nil, err
		}
		for _, s := range page.Items {
			if rec, ok := recordOf(s); ok {
				records = append(records, rec)
			}
		}
		if page.NextCursor == "" {
			return records, nil
		}
		cursor = page.NextCursor
	}
}

func (c *restClient) Find(ctx context.Context, name string) (Record, error) {
	records, err := c.List(ctx)
	if err != nil {
		return Record{}, err
	}
	// Names are not unique in Daytona; the newest sandbox wins.
	var best Record
	found := false
	for _, r := range records {
		if r.Name == name && (!found || r.CreatedAt.After(best.CreatedAt)) {
			best, found = r, true
		}
	}
	if !found {
		return Record{}, ErrNotFound
	}
	return best, nil
}

func (c *restClient) Get(ctx context.Context, id string) (Record, error) {
	var out sandboxJSON
	if err := c.do(ctx, http.MethodGet, "/sandboxes/"+url.PathEscape(id), nil, nil, &out); err != nil {
		return Record{}, notFound(err)
	}
	rec, ok := recordOf(out)
	if !ok {
		return Record{}, ErrNotFound
	}
	return rec, nil
}

// recordOf converts a sandbox. A sandbox that kaxa did not create, or that is
// being deleted, is not a record.
func recordOf(s sandboxJSON) (Record, bool) {
	if s.Labels[labelManaged] != labelTrue || s.Labels[labelName] == "" {
		return Record{}, false
	}
	if s.Status == "deleting" || s.Status == "deleted" {
		return Record{}, false
	}
	rec := Record{
		ID: s.ID, Name: s.Labels[labelName], Status: s.Status,
		CreatedAt: s.CreatedAt, ToolboxURL: s.ToolboxURL, Template: s.Labels[labelTemplate],
	}
	if raw := s.Labels[labelLabels]; raw != "" {
		rec.Labels = slices.DeleteFunc(strings.Split(raw, ","), func(l string) bool { return l == "" })
	}
	if port, err := strconv.Atoi(s.Labels[labelAPIPort]); err == nil && port > 0 {
		rec.APIPort = port
	}
	return rec, true
}

func (c *restClient) Delete(ctx context.Context, id string) error {
	return notFound(c.do(ctx, http.MethodDelete, "/sandboxes/"+url.PathEscape(id), nil, nil, nil))
}

func (c *restClient) Start(ctx context.Context, id string) error {
	return notFound(c.do(ctx, http.MethodPost, "/sandboxes/"+url.PathEscape(id)+"/start", nil, nil, nil))
}

func (c *restClient) Resume(ctx context.Context, id string) error {
	return notFound(c.do(ctx, http.MethodPost, "/sandboxes/"+url.PathEscape(id)+"/resume", nil, nil, nil))
}

func (c *restClient) Stop(ctx context.Context, id string) error {
	return notFound(c.do(ctx, http.MethodPost, "/sandboxes/"+url.PathEscape(id)+"/stop", nil, nil, nil))
}

func portPath(id string, port int) string {
	return "/sandboxes/" + url.PathEscape(id) + "/ports/" + strconv.Itoa(port)
}

func (c *restClient) SetPort(ctx context.Context, id string, port int, public bool) (PortEndpoint, error) {
	var out portJSON
	if err := c.do(ctx, http.MethodPut, portPath(id, port), nil, map[string]bool{"public": public}, &out); err != nil {
		return PortEndpoint{}, notFound(err)
	}
	return PortEndpoint(out), nil
}

func (c *restClient) GetPort(ctx context.Context, id string, port int) (PortEndpoint, error) {
	var out portJSON
	if err := c.do(ctx, http.MethodGet, portPath(id, port), nil, nil, &out); err != nil {
		return PortEndpoint{}, notFound(err)
	}
	return PortEndpoint(out), nil
}

func (c *restClient) CaptureImage(ctx context.Context, id, name string) (Image, error) {
	var out imageJSON
	if err := c.do(ctx, http.MethodPost, "/sandboxes/"+url.PathEscape(id)+"/images", nil, map[string]string{"name": name}, &out); err != nil {
		return Image{}, notFound(err)
	}
	return Image(out), nil
}

func (c *restClient) ListImages(ctx context.Context, prefix string) ([]Image, error) {
	var images []Image
	cursor := ""
	for {
		q := url.Values{"limit": {strconv.Itoa(maxPageSize)}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var page imagePage
		if err := c.do(ctx, http.MethodGet, "/images", q, nil, &page); err != nil {
			return nil, err
		}
		for _, img := range page.Items {
			if strings.HasPrefix(img.Name, prefix) {
				images = append(images, Image(img))
			}
		}
		if page.NextCursor == "" {
			return images, nil
		}
		cursor = page.NextCursor
	}
}

func (c *restClient) GetImage(ctx context.Context, name string) (Image, error) {
	var page imagePage
	if err := c.do(ctx, http.MethodGet, "/images", url.Values{"name": {name}}, nil, &page); err != nil {
		return Image{}, err
	}
	for _, img := range page.Items {
		if img.Name == name {
			return Image(img), nil
		}
	}
	return Image{}, ErrImageNotFound
}

func (c *restClient) DeleteImage(ctx context.Context, id string) error {
	err := c.do(ctx, http.MethodDelete, "/images/"+url.PathEscape(id), nil, nil, nil)
	if api, ok := errors.AsType[*APIError](err); ok && api.Status == http.StatusNotFound {
		return ErrImageNotFound
	}
	return err
}

// do sends one JSON request. A non-2xx answer is an *APIError.
func (c *restClient) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	endpoint := strings.TrimRight(c.base.String(), "/") + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return apiErrorOf(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// apiErrorOf reads the message of a failed response.
func apiErrorOf(resp *http.Response) *APIError {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	var body struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	msg := strings.TrimSpace(string(raw))
	if json.Unmarshal(raw, &body) == nil {
		if body.Message != "" {
			msg = body.Message
		} else if body.Error != "" {
			msg = body.Error
		}
	}
	return &APIError{Status: resp.StatusCode, Message: msg}
}
