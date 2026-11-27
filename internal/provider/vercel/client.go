package vercel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const paramProjectID = "projectId"

const (
	defaultBaseURL = "https://api.vercel.com"
	// requestTimeout bounds every call except the exec stream, which runs as
	// long as the command and ends with the context.
	requestTimeout = 2 * time.Minute
	// resumeTimeout bounds a resume, which blocks until the new session runs.
	// Vercel restores a snapshot that is not cached more slowly.
	resumeTimeout = 10 * time.Minute
	pageSize      = 50
	maxErrorBody  = 4 << 10

	tagManaged = "kaxa-managed"
	tagLabels  = "kaxa-labels"
	tagAPIPort = "kaxa-api-port"
	tagTrue    = "true"

	statusRunning  = "running"
	statusStopped  = "stopped"
	statusStopping = "stopping"

	snapshotCreated = "created"
	snapshotManual  = "manual"
)

// CreateParams describes the sandbox to create.
type CreateParams struct {
	Name   string
	Labels []string
	// Image is the image to start from. Empty means the Vercel default.
	Image string
	// VCPUs is the vCPU count. Zero means the platform default.
	VCPUs int
	// APIPort is the port of the API that will be published. Zero means none.
	APIPort int
	// Env holds default environment variables inherited by every command.
	Env map[string]string
	// BlockNetwork denies all outbound traffic.
	BlockNetwork bool
}

// Route maps a sandbox port to a public URL.
type Route struct {
	Port int
	URL  string
}

// Record is one Vercel named sandbox as kaxa sees it.
type Record struct {
	Name       string
	Status     string
	SessionID  string
	SnapshotID string
	Image      string
	Region     string
	Runtime    string
	Labels     []string
	// APIPort is the port of the published API, or zero.
	APIPort   int
	VCPUs     int
	MemoryMB  int
	CreatedAt time.Time
	// ExpiresAt is when the running session times out. Zero when stopped.
	ExpiresAt time.Time
	Routes    []Route
}

// Snapshot is one Vercel snapshot.
type Snapshot struct {
	ID        string
	Status    string
	Method    string
	CreatedAt time.Time
}

// CmdRequest is one command to run in a session.
type CmdRequest struct {
	Command string
	Args    []string
	Cwd     string
	Env     map[string]string
	// Stdin keeps stdin open for WriteStdin.
	Stdin bool
}

// EventKind tells what an Event carries.
type EventKind int

// Event kinds of a command stream.
const (
	EventStarted EventKind = iota + 1
	EventStdout
	EventStderr
	EventError
	EventEnd
)

// Event is one item of a command stream.
type Event struct {
	Kind  EventKind
	Data  []byte
	CmdID string
	// ExitCode is set on EventEnd. Nil means Vercel recorded no exit code.
	ExitCode *int
	Err      string
}

// Stream yields the events of one command until EventEnd.
type Stream interface {
	Next() (Event, error)
	Close() error
}

type apiClient interface {
	Create(ctx context.Context, p CreateParams) (Record, error)
	List(ctx context.Context) ([]Record, error)
	// Get reads a named sandbox. resume starts a session when it is stopped.
	Get(ctx context.Context, name string, resume bool) (Record, error)
	Delete(ctx context.Context, name string) error
	Update(ctx context.Context, name string, patch map[string]any) error
	StopSession(ctx context.Context, sessionID string) error
	ExtendTimeout(ctx context.Context, sessionID string, d time.Duration) error
	// Run starts a command and streams its output until it ends.
	Run(ctx context.Context, sessionID string, req CmdRequest) (Stream, error)
	// Start starts a command and returns at once with its ID.
	Start(ctx context.Context, sessionID string, req CmdRequest) (string, error)
	WriteStdin(ctx context.Context, sessionID, cmdID string, data []byte, offset int64, closeStdin bool) error
	Kill(ctx context.Context, sessionID, cmdID string) error
	Mkdir(ctx context.Context, sessionID, dir string) error
	WriteTar(ctx context.Context, sessionID, dir string, tarGz io.Reader) error
	CreateSnapshot(ctx context.Context, sessionID string) (Snapshot, error)
	ListSnapshots(ctx context.Context, name string) ([]Snapshot, error)
	DeleteSnapshot(ctx context.Context, id string) error
	// OpenInteractive asks the controller for a terminal and returns where to
	// connect.
	OpenInteractive(ctx context.Context, sessionID string) (Interactive, error)
}

// Interactive is the WebSocket endpoint of a controller-hosted terminal. The
// token is a credential: it is never logged or printed.
type Interactive struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

type restClient struct {
	base    string
	token   string
	project string
	team    string
	http    *http.Client
	// timeout bounds a normal call; resumeTimeout bounds a resume.
	timeout, resumeTimeout time.Duration
}

func newRESTClient(base, token, project, team string) *restClient {
	return &restClient{
		base: base, token: token, project: project, team: team, http: &http.Client{},
		timeout: requestTimeout, resumeTimeout: resumeTimeout,
	}
}

type wireSandbox struct {
	Name              string            `json:"name"`
	Status            string            `json:"status"`
	CurrentSessionID  string            `json:"currentSessionId"`
	CurrentSnapshotID string            `json:"currentSnapshotId"`
	Image             string            `json:"image"`
	Region            string            `json:"region"`
	Runtime           string            `json:"runtime"`
	VCPUs             int               `json:"vcpus"`
	Memory            int               `json:"memory"`
	CreatedAt         int64             `json:"createdAt"`
	ExpiresAt         int64             `json:"expiresAt"`
	Tags              map[string]string `json:"tags"`
}

type wireEnvelope struct {
	Sandbox wireSandbox `json:"sandbox"`
	Session *struct {
		ID string `json:"id"`
	} `json:"session"`
	Routes []struct {
		Port int    `json:"port"`
		URL  string `json:"url"`
	} `json:"routes"`
}

type wirePage struct {
	Pagination struct {
		Next *string `json:"next"`
	} `json:"pagination"`
	Sandboxes []wireSandbox `json:"sandboxes"`
	Snapshots []struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		Method    string `json:"creationMethod"`
		CreatedAt int64  `json:"createdAt"`
	} `json:"snapshots"`
}

func millis(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

// recordOf converts a sandbox. ok is false for a sandbox kaxa did not create.
func recordOf(s wireSandbox) (Record, bool) {
	if s.Tags[tagManaged] != tagTrue || s.Name == "" {
		return Record{}, false
	}
	rec := Record{
		Name: s.Name, Status: s.Status, SessionID: s.CurrentSessionID, SnapshotID: s.CurrentSnapshotID,
		Image: s.Image, Region: s.Region, Runtime: s.Runtime, VCPUs: s.VCPUs, MemoryMB: s.Memory,
		CreatedAt: millis(s.CreatedAt), ExpiresAt: millis(s.ExpiresAt),
	}
	if raw := s.Tags[tagLabels]; raw != "" {
		rec.Labels = slices.DeleteFunc(strings.Split(raw, ","), func(l string) bool { return l == "" })
	}
	if port, err := strconv.Atoi(s.Tags[tagAPIPort]); err == nil && port > 0 {
		rec.APIPort = port
	}
	return rec, true
}

func (e wireEnvelope) record() (Record, error) {
	rec, ok := recordOf(e.Sandbox)
	if !ok {
		return Record{}, ErrNotFound
	}
	if e.Session != nil && e.Session.ID != "" {
		rec.SessionID = e.Session.ID
	}
	for _, r := range e.Routes {
		rec.Routes = append(rec.Routes, Route{Port: r.Port, URL: r.URL})
	}
	return rec, nil
}

func tagsOf(p CreateParams) map[string]string {
	tags := map[string]string{tagManaged: tagTrue, tagLabels: strings.Join(p.Labels, ",")}
	if p.APIPort > 0 {
		tags[tagAPIPort] = strconv.Itoa(p.APIPort)
	}
	return tags
}

func (c *restClient) Create(ctx context.Context, p CreateParams) (Record, error) {
	body := map[string]any{
		"name":         p.Name,
		paramProjectID: c.project,
		"timeout":      sessionTimeout.Milliseconds(),
		// Automatic snapshots must not evict the manual ones.
		"keepLastSnapshots": nil,
		"persistent":        true,
		"tags":              tagsOf(p),
	}
	if p.Image != "" {
		body["image"] = p.Image
	}
	if p.VCPUs > 0 {
		body["resources"] = map[string]int{"vcpus": p.VCPUs, "memory": p.VCPUs * 2048}
	}
	if p.APIPort > 0 {
		body["ports"] = []int{p.APIPort}
	}
	if len(p.Env) > 0 {
		body["env"] = p.Env
	}
	if p.BlockNetwork {
		body["networkPolicy"] = map[string]string{"mode": "deny-all"}
	}
	var out wireEnvelope
	if _, err := c.do(ctx, http.MethodPost, "/v4/sandboxes", nil, body, &out); err != nil {
		if api, ok := errors.AsType[*APIError](err); ok && api.Status == http.StatusConflict {
			return Record{}, ErrAlreadyExists
		}
		return Record{}, err
	}
	rec, err := out.record()
	if err != nil {
		return Record{}, errors.New("vercel: create returned no managed sandbox")
	}
	return rec, nil
}

func (c *restClient) List(ctx context.Context) ([]Record, error) {
	var records []Record
	cursor := ""
	for {
		q := url.Values{
			"project": {c.project},
			"limit":   {strconv.Itoa(pageSize)},
			"tags":    {tagManaged + ":" + tagTrue},
		}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var page wirePage
		if _, err := c.do(ctx, http.MethodGet, "/v2/sandboxes", q, nil, &page); err != nil {
			return nil, err
		}
		// The tag filter is checked again, so a server that ignores it still
		// cannot hand back another tool's sandboxes.
		for _, s := range page.Sandboxes {
			if rec, ok := recordOf(s); ok {
				records = append(records, rec)
			}
		}
		if page.Pagination.Next == nil || *page.Pagination.Next == "" {
			return records, nil
		}
		cursor = *page.Pagination.Next
	}
}

func (c *restClient) named(name string) string {
	return "/v2/sandboxes/" + url.PathEscape(name)
}

func (c *restClient) Get(ctx context.Context, name string, resume bool) (Record, error) {
	q := url.Values{paramProjectID: {c.project}, "resume": {strconv.FormatBool(resume)}}
	var out wireEnvelope
	limit := c.timeout
	if resume {
		limit = c.resumeTimeout
	}
	if _, err := c.doWithin(ctx, limit, http.MethodGet, c.named(name), q, nil, &out); err != nil {
		return Record{}, notFound(err)
	}
	return out.record()
}

func (c *restClient) Delete(ctx context.Context, name string) error {
	q := url.Values{paramProjectID: {c.project}, "deleteOrphanSnapshots": {"true"}}
	_, err := c.do(ctx, http.MethodDelete, c.named(name), q, nil, nil)
	return notFound(err)
}

func (c *restClient) Update(ctx context.Context, name string, patch map[string]any) error {
	q := url.Values{paramProjectID: {c.project}}
	_, err := c.do(ctx, http.MethodPatch, c.named(name), q, patch, nil)
	return notFound(err)
}

func sessionPath(id, tail string) string {
	return "/v2/sandboxes/sessions/" + url.PathEscape(id) + tail
}

func (c *restClient) StopSession(ctx context.Context, sessionID string) error {
	_, err := c.do(ctx, http.MethodPost, sessionPath(sessionID, "/stop"), nil, map[string]any{}, nil)
	return notFound(err)
}

func (c *restClient) ExtendTimeout(ctx context.Context, sessionID string, d time.Duration) error {
	body := map[string]int64{"duration": d.Milliseconds()}
	_, err := c.do(ctx, http.MethodPost, sessionPath(sessionID, "/extend-timeout"), nil, body, nil)
	return notFound(err)
}

func (c *restClient) OpenInteractive(ctx context.Context, sessionID string) (Interactive, error) {
	var out Interactive
	if _, err := c.do(ctx, http.MethodPost, sessionPath(sessionID, "/interactive"), nil, map[string]any{}, &out); err != nil {
		return Interactive{}, err
	}
	return out, nil
}

func (c *restClient) Mkdir(ctx context.Context, sessionID, dir string) error {
	body := map[string]any{"path": dir, "recursive": true}
	_, err := c.do(ctx, http.MethodPost, sessionPath(sessionID, "/fs/mkdir"), nil, body, nil)
	return notFound(err)
}

func (c *restClient) WriteTar(ctx context.Context, sessionID, dir string, tarGz io.Reader) error {
	// No request timeout: the archive can be large and is streamed.
	req, err := c.newRequest(ctx, http.MethodPost, sessionPath(sessionID, "/fs/write"), nil, tarGz)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/gzip")
	req.Header.Set("x-cwd", dir)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return notFound(apiErrorOf(resp))
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBody))
	return nil
}

func cmdBody(req CmdRequest) map[string]any {
	body := map[string]any{"command": req.Command, "args": req.Args}
	if req.Args == nil {
		body["args"] = []string{}
	}
	if req.Cwd != "" {
		body["cwd"] = req.Cwd
	}
	if len(req.Env) > 0 {
		body["env"] = req.Env
	}
	if req.Stdin {
		body["attachStdin"] = true
	}
	return body
}

func (c *restClient) Start(ctx context.Context, sessionID string, req CmdRequest) (string, error) {
	var out struct {
		Command struct {
			ID string `json:"id"`
		} `json:"command"`
	}
	if _, err := c.do(ctx, http.MethodPost, sessionPath(sessionID, "/cmd"), nil, cmdBody(req), &out); err != nil {
		return "", notFound(err)
	}
	if out.Command.ID == "" {
		return "", errors.New("vercel: command returned no ID")
	}
	return out.Command.ID, nil
}

func (c *restClient) Run(ctx context.Context, sessionID string, req CmdRequest) (Stream, error) {
	body := cmdBody(req)
	body["wait"], body["logs"] = true, true
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.newRequest(ctx, http.MethodPost, sessionPath(sessionID, "/cmd"), nil, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", cborStreamType)
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		defer func() { _ = resp.Body.Close() }()
		return nil, notFound(apiErrorOf(resp))
	}
	return newCBORStream(resp.Body), nil
}

func (c *restClient) WriteStdin(ctx context.Context, sessionID, cmdID string, data []byte, offset int64, closeStdin bool) error {
	body := map[string]any{"data": base64.StdEncoding.EncodeToString(data), "offset": offset, "close": closeStdin}
	path := sessionPath(sessionID, "/cmd/"+url.PathEscape(cmdID)+"/stdin")
	_, err := c.do(ctx, http.MethodPost, path, nil, body, nil)
	return notFound(err)
}

func (c *restClient) Kill(ctx context.Context, sessionID, cmdID string) error {
	path := sessionPath(sessionID, "/cmd/"+url.PathEscape(cmdID)+"/kill")
	_, err := c.do(ctx, http.MethodPost, path, nil, map[string]int{"signal": 9}, nil)
	return notFound(err)
}

func (p wirePage) snapshots() []Snapshot {
	out := make([]Snapshot, 0, len(p.Snapshots))
	for _, s := range p.Snapshots {
		out = append(out, Snapshot{ID: s.ID, Status: s.Status, Method: s.Method, CreatedAt: millis(s.CreatedAt)})
	}
	return out
}

func (c *restClient) CreateSnapshot(ctx context.Context, sessionID string) (Snapshot, error) {
	var out struct {
		Snapshot struct {
			ID        string `json:"id"`
			Status    string `json:"status"`
			Method    string `json:"creationMethod"`
			CreatedAt int64  `json:"createdAt"`
		} `json:"snapshot"`
	}
	// Expiration zero keeps a manual snapshot until it is deleted.
	body := map[string]int{"expiration": 0}
	if _, err := c.do(ctx, http.MethodPost, "/v3/sandboxes/sessions/"+url.PathEscape(sessionID)+"/snapshot", nil, body, &out); err != nil {
		return Snapshot{}, notFound(err)
	}
	s := out.Snapshot
	if s.ID == "" {
		return Snapshot{}, errors.New("vercel: snapshot returned no ID")
	}
	return Snapshot{ID: s.ID, Status: s.Status, Method: s.Method, CreatedAt: millis(s.CreatedAt)}, nil
}

func (c *restClient) ListSnapshots(ctx context.Context, name string) ([]Snapshot, error) {
	var all []Snapshot
	cursor := ""
	for {
		q := url.Values{"project": {c.project}, "name": {name}, "limit": {strconv.Itoa(pageSize)}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		var page wirePage
		if _, err := c.do(ctx, http.MethodGet, "/v2/sandboxes/snapshots", q, nil, &page); err != nil {
			return nil, notFound(err)
		}
		all = append(all, page.snapshots()...)
		if page.Pagination.Next == nil || *page.Pagination.Next == "" {
			return all, nil
		}
		cursor = *page.Pagination.Next
	}
}

func (c *restClient) DeleteSnapshot(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, "/v2/sandboxes/snapshots/"+url.PathEscape(id), nil, nil, nil)
	return notFound(err)
}

func notFound(err error) error {
	if api, ok := errors.AsType[*APIError](err); ok && api.Status == http.StatusNotFound {
		return ErrNotFound
	}
	return err
}

// newRequest builds a request to the fixed API base. The token travels only in
// the Authorization header, never in the URL.
func (c *restClient) newRequest(ctx context.Context, method, path string, query url.Values, body io.Reader) (*http.Request, error) {
	q := url.Values{}
	maps.Copy(q, query)
	if strings.HasPrefix(c.team, "team_") {
		q.Set("teamId", c.team)
	} else if c.team != "" {
		q.Set("slug", c.team)
	}
	endpoint := strings.TrimRight(c.base, "/") + path
	if len(q) > 0 {
		endpoint += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	return req, nil
}

// do sends one JSON request. A non-2xx answer is an *APIError.
func (c *restClient) do(ctx context.Context, method, path string, query url.Values, body, out any) (http.Header, error) {
	return c.doWithin(ctx, c.timeout, method, path, query, body, out)
}

// doWithin is do with its own time limit.
func (c *restClient) doWithin(ctx context.Context, limit time.Duration, method, path string, query url.Values, body, out any) (http.Header, error) {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := c.newRequest(ctx, method, path, query, reader)
	if err != nil {
		return nil, err
	}
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

// apiErrorOf reads the message of a failed response. Vercel answers with
// {"error": {"code": ..., "message": "..."}}.
func apiErrorOf(resp *http.Response) *APIError {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	msg := strings.TrimSpace(string(raw))
	if json.Unmarshal(raw, &body) == nil && body.Error.Message != "" {
		msg = body.Error.Message
	}
	return &APIError{Status: resp.StatusCode, Message: msg}
}
