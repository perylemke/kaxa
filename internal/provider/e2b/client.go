package e2b

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
	defaultAPIURL = "https://api.e2b.app"
	defaultDomain = "e2b.app"
	// defaultTemplate is E2B's default template: Debian with git, curl, Python
	// and Node, with 2 vCPU and 512 MiB of memory.
	defaultTemplate = "base"
	// sandboxTimeout is how long a sandbox runs after the last kaxa command.
	// Then E2B pauses it with its filesystem and memory, and the next command
	// resumes it. One hour is the longest a Hobby account can run.
	sandboxTimeout = 3600
	maxPageSize    = 100
	maxErrorBody   = 4 << 10

	metaManaged  = "kaxa.managed"
	metaName     = "kaxa.name"
	metaLabels   = "kaxa.labels"
	metaAPIPort  = "kaxa.api_port"
	metaTemplate = "kaxa.template"
	metaTrue     = "true"

	stateRunning = "running"
	statePaused  = "paused"
)

// CreateParams describes the sandbox to create.
type CreateParams struct {
	Name   string
	Labels []string
	// Template is the template to start from. Empty means the base template.
	Template string
	// APIPort is the port of the API that will be published. Zero means none.
	APIPort int
	// PublicTraffic lets anyone with the sandbox URLs reach its services. It
	// cannot change after creation, so it is on only for a published API.
	PublicTraffic bool
}

// ConnectParams are the options of resuming or reaching a sandbox.
type ConnectParams struct {
	// Reboot resumes a paused sandbox from its disk, dropping its memory.
	Reboot bool
}

// Record is one E2B sandbox as kaxa sees it.
type Record struct {
	// ID is the identifier E2B generated. Name is the kaxa name.
	ID        string
	Name      string
	Labels    []string
	State     string
	StartedAt time.Time
	// APIPort is the port of the published API, or zero.
	APIPort int
	// Template is the template the sandbox was created from. Empty means base.
	Template string
}

// Connection is what the sandbox daemon needs to accept a request.
type Connection struct {
	ID          string
	Domain      string
	AccessToken string
}

type apiClient interface {
	// Create starts a sandbox and returns its E2B ID.
	Create(ctx context.Context, p CreateParams) (string, error)
	// List returns the sandboxes kaxa manages, running or paused.
	List(ctx context.Context) ([]Record, error)
	// Find returns the sandbox called name, or ErrNotFound.
	Find(ctx context.Context, name string) (Record, error)
	Kill(ctx context.Context, id string) error
	// Pause stops a sandbox and keeps its state. memory false keeps only the disk.
	Pause(ctx context.Context, id string, memory bool) error
	// Connect resumes a paused sandbox, extends its timeout, and returns what
	// the daemon needs.
	Connect(ctx context.Context, id string, p ConnectParams) (Connection, error)
}

type restClient struct {
	base   string
	domain string
	key    string
	http   *http.Client
}

func newRESTClient(apiKey string) *restClient {
	return &restClient{
		base:   defaultAPIURL,
		domain: defaultDomain,
		key:    apiKey,
		http:   &http.Client{Timeout: 2 * time.Minute},
	}
}

type sandboxResponse struct {
	SandboxID   string  `json:"sandboxID"`
	AccessToken string  `json:"envdAccessToken"`
	Domain      *string `json:"domain"`
}

type listedSandbox struct {
	SandboxID string            `json:"sandboxID"`
	StartedAt time.Time         `json:"startedAt"`
	State     string            `json:"state"`
	Metadata  map[string]string `json:"metadata"`
}

func (c *restClient) Create(ctx context.Context, p CreateParams) (string, error) {
	meta := map[string]string{metaManaged: metaTrue, metaName: p.Name, metaLabels: strings.Join(p.Labels, ",")}
	if p.APIPort > 0 {
		meta[metaAPIPort] = strconv.Itoa(p.APIPort)
	}
	template := defaultTemplate
	if p.Template != "" {
		template = p.Template
		meta[metaTemplate] = p.Template
	}
	body := map[string]any{
		"templateID": template,
		"timeout":    sandboxTimeout,
		"autoPause":  true,
		"metadata":   meta,
		"network":    map[string]any{"allowPublicTraffic": p.PublicTraffic},
	}
	var out sandboxResponse
	if _, err := c.do(ctx, http.MethodPost, "/v2/sandboxes", nil, body, &out); err != nil {
		if p.Template != "" {
			if api, ok := errors.AsType[*APIError](err); ok && api.Status == http.StatusNotFound {
				return "", ErrTemplateNotFound
			}
		}
		return "", err
	}
	if out.SandboxID == "" {
		return "", errors.New("e2b: create returned no sandbox ID")
	}
	return out.SandboxID, nil
}

func (c *restClient) List(ctx context.Context) ([]Record, error) {
	return c.list(ctx, "")
}

func (c *restClient) Find(ctx context.Context, name string) (Record, error) {
	records, err := c.list(ctx, name)
	if err != nil {
		return Record{}, err
	}
	if len(records) == 0 {
		return Record{}, ErrNotFound
	}
	return records[0], nil
}

// list pages through the managed sandboxes. A non-empty name narrows the query
// to that sandbox. The result is filtered again here, so a server that ignores
// the metadata filter still cannot return another tool's sandboxes.
func (c *restClient) list(ctx context.Context, name string) ([]Record, error) {
	filter := queryPair(metaManaged, metaTrue)
	if name != "" {
		filter += "&" + queryPair(metaName, name)
	}
	var records []Record
	token := ""
	for {
		q := url.Values{
			"metadata": {filter},
			"state":    {stateRunning + "," + statePaused},
			"limit":    {strconv.Itoa(maxPageSize)},
		}
		if token != "" {
			q.Set("nextToken", token)
		}
		var page []listedSandbox
		header, err := c.do(ctx, http.MethodGet, "/v2/sandboxes", q, nil, &page)
		if err != nil {
			return nil, err
		}
		for _, s := range page {
			if rec, ok := recordOf(s); ok && (name == "" || rec.Name == name) {
				records = append(records, rec)
			}
		}
		token = header.Get("X-Next-Token")
		if token == "" {
			return records, nil
		}
	}
}

func queryPair(key, value string) string {
	return url.QueryEscape(key) + "=" + url.QueryEscape(value)
}

func recordOf(s listedSandbox) (Record, bool) {
	if s.Metadata[metaManaged] != metaTrue || s.Metadata[metaName] == "" {
		return Record{}, false
	}
	rec := Record{ID: s.SandboxID, Name: s.Metadata[metaName], State: s.State, StartedAt: s.StartedAt}
	if raw := s.Metadata[metaLabels]; raw != "" {
		rec.Labels = slices.DeleteFunc(strings.Split(raw, ","), func(l string) bool { return l == "" })
	}
	if port, err := strconv.Atoi(s.Metadata[metaAPIPort]); err == nil && port > 0 {
		rec.APIPort = port
	}
	rec.Template = s.Metadata[metaTemplate]
	return rec, true
}

func (c *restClient) Kill(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, "/sandboxes/"+url.PathEscape(id), nil, nil, nil)
	return notFound(err)
}

func (c *restClient) Pause(ctx context.Context, id string, memory bool) error {
	_, err := c.do(ctx, http.MethodPost, "/sandboxes/"+url.PathEscape(id)+"/pause", nil, map[string]bool{"memory": memory}, nil)
	return notFound(err)
}

func (c *restClient) Connect(ctx context.Context, id string, p ConnectParams) (Connection, error) {
	body := map[string]any{"timeout": sandboxTimeout}
	if p.Reboot {
		body["memory"] = false
	}
	var out sandboxResponse
	if _, err := c.do(ctx, http.MethodPost, "/v2/sandboxes/"+url.PathEscape(id)+"/connect", nil, body, &out); err != nil {
		return Connection{}, notFound(err)
	}
	if out.AccessToken == "" {
		return Connection{}, errors.New("e2b: connect returned no access token")
	}
	domain := c.domain
	if out.Domain != nil && *out.Domain != "" {
		domain = *out.Domain
	}
	return Connection{ID: id, Domain: domain, AccessToken: out.AccessToken}, nil
}

func notFound(err error) error {
	if api, ok := errors.AsType[*APIError](err); ok && api.Status == http.StatusNotFound {
		return ErrNotFound
	}
	return err
}

// do sends one JSON request. A non-2xx answer is an *APIError.
func (c *restClient) do(ctx context.Context, method, path string, query url.Values, body, out any) (http.Header, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}
	endpoint := strings.TrimRight(c.base, "/") + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-API-Key", c.key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, apiErrorOf(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return resp.Header, nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return resp.Header, nil
}

// apiErrorOf reads the message of a failed response. Both E2B APIs answer with
// {"code": ..., "message": "..."}.
func apiErrorOf(resp *http.Response) *APIError {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	var body struct {
		Message string `json:"message"`
	}
	msg := strings.TrimSpace(string(raw))
	if json.Unmarshal(raw, &body) == nil && body.Message != "" {
		msg = body.Message
	}
	return &APIError{Status: resp.StatusCode, Message: msg}
}
