package docker

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://connect.docker.com/sandboxes"

	// maxResponseBody caps what kaxa reads from one API response.
	maxResponseBody = 4 << 20
	pageSize        = "100"

	idempotencyHeader = "Idempotency-Key"

	managedLabel = "kaxa.managed"
	labelPrefix  = "kaxa.label."
	labelTrue    = "true"

	// shellImage is the image a sandbox starts from when no template is given.
	// It is the image of the Docker shell agent.
	shellImage = "docker/sandbox-templates:shell"
	// sandboxTTL is how long a sandbox runs before Docker stops it. It is
	// renewed on every command, and onTimeout is stop, never delete, so work
	// is never lost to an expiry.
	sandboxTTL = time.Hour
)

// Sandbox statuses of the API.
const (
	statusRunning = "running"
	statusStopped = "stopped"
	statusFailed  = "failed"
)

// Endpoint locates the process and file API of one sandbox.
type Endpoint struct {
	URI string
}

// PublishedPort is a sandbox port that Docker made reachable from outside.
type PublishedPort struct {
	Port int
	URL  string
}

// Record is a sandbox as kaxa sees it.
type Record struct {
	// Name is the display name, the name kaxa uses for the sandbox.
	Name string
	// Resource is the API resource name, such as sandboxes/abc.
	Resource  string
	Status    string
	Failure   string
	Labels    []string
	Image     string
	CreatedAt time.Time
	Etag      string
	Endpoint  Endpoint
	Ports     []PublishedPort
	managed   bool
}

// Managed reports whether kaxa created the sandbox.
func (r Record) Managed() bool { return r.managed }

// CreateParams describes a sandbox to create.
type CreateParams struct {
	Name   string
	Labels []string
	Image  string
}

// Credential is a short-lived token for the process API of one sandbox.
type Credential struct {
	Token string
}

// apiClient is the management API of Docker Sandboxes. client.go implements it
// over REST; tests use a fake.
type apiClient interface {
	List(ctx context.Context) ([]Record, error)
	Find(ctx context.Context, name string) (Record, error)
	Get(ctx context.Context, resource string) (Record, error)
	Create(ctx context.Context, p CreateParams) (Record, error)
	Start(ctx context.Context, r Record) error
	Stop(ctx context.Context, r Record) error
	Delete(ctx context.Context, r Record) error
	Renew(ctx context.Context, r Record, ttl time.Duration) error
	Credential(ctx context.Context, r Record) (Credential, error)
	// PublishPort makes a port of the sandbox reachable from outside and
	// returns its URL. A port that is already published returns its URL.
	PublishPort(ctx context.Context, r Record, port int) (string, error)
	snapshotAPI
	secrets() []string
}

// restClient talks to the Sandboxes REST API.
type restClient struct {
	base string
	http *http.Client
	auth *authenticator
	// restored returns the labels of the sandboxes kaxa restored from a
	// snapshot, by resource name. Docker gives those no labels, so without it
	// they would not be kaxa's.
	restored func() (map[string][]string, error)
}

func newRestClient(id, pat string, hc *http.Client) *restClient {
	return &restClient{base: defaultBaseURL, http: hc, auth: newAuthenticator(id, pat, hc)}
}

func (c *restClient) secrets() []string { return c.auth.secrets() }

// request is one call to the API.
type request struct {
	method  string
	url     string
	token   string
	query   url.Values
	headers map[string]string
	in      any
	out     any
	// etag receives the ETag header of the response.
	etag *string
}

// newRequest builds the HTTP request of r.
func newRequest(ctx context.Context, r request) (*http.Request, error) {
	var body io.Reader
	if r.in != nil {
		raw, err := json.Marshal(r.in)
		if err != nil {
			return nil, fmt.Errorf("docker: encode request: %w", err)
		}
		body = bytes.NewReader(raw)
	}
	target := r.url
	if len(r.query) > 0 {
		target += "?" + r.query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, r.method, target, body)
	if err != nil {
		return nil, fmt.Errorf("docker: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Accept", "application/json")
	if r.in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range r.headers {
		req.Header.Set(k, v)
	}
	return req, nil
}

// roundTrip sends a JSON request and decodes a JSON response. Statuses outside
// 2xx become *APIError.
func roundTrip(ctx context.Context, hc *http.Client, r request) error {
	req, err := newRequest(ctx, r)
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("docker: %s %s: %w", r.method, req.URL.Path, urlErrorCause(err))
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return fmt.Errorf("docker: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return parseAPIError(resp.StatusCode, raw)
	}
	if r.etag != nil {
		*r.etag = resp.Header.Get("ETag")
	}
	if r.out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, r.out); err != nil {
		return fmt.Errorf("docker: decode response: %w", err)
	}
	return nil
}

// urlErrorCause drops the *url.Error wrapper, which repeats the request URL.
func urlErrorCause(err error) error {
	if ue, ok := errors.AsType[*url.Error](err); ok {
		return ue.Err
	}
	return err
}

type wireError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func parseAPIError(status int, raw []byte) *APIError {
	var we wireError
	if err := json.Unmarshal(raw, &we); err != nil {
		return &APIError{Status: status}
	}
	return &APIError{Status: status, Code: we.Code, Message: we.Message}
}

// wireSandbox is the part of the Sandbox resource kaxa reads.
type wireSandbox struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Core        struct {
		Status    string            `json:"status"`
		Labels    map[string]string `json:"labels"`
		ImageRef  string            `json:"imageRef"`
		CreatedAt string            `json:"createdAt"`
		Etag      string            `json:"etag"`
		Endpoint  struct {
			URI string `json:"uri"`
		} `json:"endpoint"`
		Ports []struct {
			SandboxPort int    `json:"sandboxPort"`
			URL         string `json:"url"`
		} `json:"ports"`
	} `json:"core"`
	Failure *wireError `json:"failure"`
}

func (w wireSandbox) record(headerEtag string) Record {
	r := Record{
		Name:     w.DisplayName,
		Resource: w.Name,
		Status:   w.Core.Status,
		Image:    w.Core.ImageRef,
		Etag:     w.Core.Etag,
		Endpoint: Endpoint{URI: w.Core.Endpoint.URI},
		managed:  w.Core.Labels[managedLabel] == labelTrue,
	}
	if headerEtag != "" {
		r.Etag = headerEtag
	}
	if w.Failure != nil {
		r.Failure = w.Failure.Message
		if r.Failure == "" {
			r.Failure = w.Failure.Code
		}
	}
	for key := range w.Core.Labels {
		if label, ok := strings.CutPrefix(key, labelPrefix); ok {
			r.Labels = append(r.Labels, label)
		}
	}
	slices.Sort(r.Labels)
	for _, p := range w.Core.Ports {
		r.Ports = append(r.Ports, PublishedPort{Port: p.SandboxPort, URL: p.URL})
	}
	if t, err := time.Parse(time.RFC3339Nano, w.Core.CreatedAt); err == nil {
		r.CreatedAt = t
	}
	return r
}

type listResponse struct {
	Sandboxes     []wireSandbox `json:"sandboxes"`
	NextPageToken string        `json:"nextPageToken"`
}

// maxPages bounds a listing so a server that never ends the pages cannot
// keep kaxa busy.
const maxPages = 50

func (c *restClient) call(ctx context.Context, r request) error {
	token, err := c.auth.Token(ctx)
	if err != nil {
		return err
	}
	r.token = token
	return roundTrip(ctx, c.http, r)
}

func (c *restClient) page(ctx context.Context, filter, token string) (listResponse, error) {
	q := url.Values{"pageSize": {pageSize}}
	if filter != "" {
		q.Set("filter", filter)
	}
	if token != "" {
		q.Set("pageToken", token)
	}
	var out listResponse
	err := c.call(ctx, request{method: http.MethodGet, url: c.base + "/v1/sandboxes", query: q, out: &out})
	return out, err
}

// adopter returns a function that marks a restored sandbox as kaxa's and gives
// it the labels of its source. Every other record is returned as it is.
func (c *restClient) adopter() (func(Record) Record, error) {
	if c.restored == nil {
		return func(r Record) Record { return r }, nil
	}
	known, err := c.restored()
	if err != nil {
		return nil, err
	}
	return func(r Record) Record {
		if labels, ok := known[r.Resource]; ok && !r.managed {
			r.managed = true
			r.Labels = slices.Clone(labels)
		}
		return r
	}, nil
}

func (c *restClient) list(ctx context.Context, filter string) ([]Record, error) {
	adopt, err := c.adopter()
	if err != nil {
		return nil, err
	}
	var records []Record
	token := ""
	for range maxPages {
		out, err := c.page(ctx, filter, token)
		if err != nil {
			return nil, err
		}
		for _, w := range out.Sandboxes {
			records = append(records, adopt(w.record("")))
		}
		if out.NextPageToken == "" {
			return records, nil
		}
		token = out.NextPageToken
	}
	return records, nil
}

// List returns every sandbox kaxa created.
func (c *restClient) List(ctx context.Context) ([]Record, error) {
	all, err := c.list(ctx, "")
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(all, func(r Record) bool { return !r.Managed() }), nil
}

// Find returns the sandbox kaxa created with that display name.
func (c *restClient) Find(ctx context.Context, name string) (Record, error) {
	// The filter narrows the listing; the exact match below decides.
	all, err := c.list(ctx, "display_name="+name)
	if err != nil {
		return Record{}, err
	}
	for _, r := range all {
		if r.Name == name && r.Managed() {
			return r, nil
		}
	}
	return Record{}, ErrNotFound
}

// Get reads one sandbox by resource name.
func (c *restClient) Get(ctx context.Context, resource string) (Record, error) {
	var w wireSandbox
	var etag string
	err := c.call(ctx, request{method: http.MethodGet, url: c.base + "/v1/" + resource, out: &w, etag: &etag})
	if isNotFound(err) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, err
	}
	adopt, err := c.adopter()
	if err != nil {
		return Record{}, err
	}
	return adopt(w.record(etag)), nil
}

type createBody struct {
	DisplayName      string            `json:"displayName"`
	ImageRef         string            `json:"imageRef"`
	Resources        resources         `json:"resources"`
	Labels           map[string]string `json:"labels"`
	StartupExecution startupExecution  `json:"startupExecution"`
	Features         features          `json:"features"`
}

// resources is the CPU and memory of a sandbox. A REST create from an image
// must send both, and Docker accepts only certain pairs.
type resources struct {
	CPUs      int `json:"cpus"`
	MemoryMiB int `json:"memoryMib"`
}

// defaultResources is the small compute size: 2 CPUs and 4 GiB.
var defaultResources = resources{CPUs: 2, MemoryMiB: 4096}

type startupExecution struct {
	RawImage struct {
		Start struct {
			Argv []string `json:"argv"`
		} `json:"start"`
	} `json:"rawImage"`
}

type features struct {
	Timeouts struct {
		Timeout   string `json:"timeout"`
		OnTimeout string `json:"onTimeout"`
	} `json:"timeouts"`
}

func newCreateBody(p CreateParams) createBody {
	labels := map[string]string{managedLabel: labelTrue}
	for _, l := range p.Labels {
		labels[labelPrefix+l] = labelTrue
	}
	b := createBody{DisplayName: p.Name, ImageRef: p.Image, Resources: defaultResources, Labels: labels}
	// The image runs a command that does nothing, so the sandbox stays up for
	// kaxa exec and kaxa console instead of exiting with the image's command.
	b.StartupExecution.RawImage.Start.Argv = []string{"tail", "-f", "/dev/null"}
	b.Features.Timeouts.Timeout = durationString(sandboxTTL)
	b.Features.Timeouts.OnTimeout = "stop"
	return b
}

func durationString(d time.Duration) string {
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

// Create asks Docker for a sandbox. The idempotency key lets a retry after a
// lost response return the first sandbox instead of creating a second one.
func (c *restClient) Create(ctx context.Context, p CreateParams) (Record, error) {
	key, err := idempotencyKey()
	if err != nil {
		return Record{}, err
	}
	var w wireSandbox
	var etag string
	err = c.call(ctx, request{
		method:  http.MethodPost,
		url:     c.base + "/v1/sandboxes",
		headers: map[string]string{idempotencyHeader: key},
		in:      newCreateBody(p),
		out:     &w,
		etag:    &etag,
	})
	if apiErr, ok := errors.AsType[*APIError](err); ok && apiErr.is(codeAlreadyExists, http.StatusConflict) {
		return Record{}, ErrAlreadyExists
	}
	if err != nil {
		return Record{}, err
	}
	return w.record(etag), nil
}

// isNotFound reports whether err is the API saying the resource does not exist.
func isNotFound(err error) bool {
	apiErr, ok := errors.AsType[*APIError](err)
	return ok && apiErr.is(codeNotFound, http.StatusNotFound)
}

func idempotencyKey() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("docker: create idempotency key: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func (c *restClient) action(ctx context.Context, r Record, verb string) error {
	return c.call(ctx, request{
		method:  http.MethodPost,
		url:     c.base + "/v1/" + r.Resource + "/" + verb,
		headers: map[string]string{"If-Match": r.Etag},
		in:      struct{}{},
	})
}

// Start starts a stopped sandbox.
func (c *restClient) Start(ctx context.Context, r Record) error { return c.action(ctx, r, "start") }

// Stop stops a running sandbox.
func (c *restClient) Stop(ctx context.Context, r Record) error { return c.action(ctx, r, "stop") }

// Delete deletes a sandbox, running or not.
func (c *restClient) Delete(ctx context.Context, r Record) error {
	err := c.call(ctx, request{
		method:  http.MethodDelete,
		url:     c.base + "/v1/" + r.Resource,
		query:   url.Values{"force": {"true"}},
		headers: map[string]string{"If-Match": r.Etag},
	})
	if apiErr, ok := errors.AsType[*APIError](err); ok && apiErr.is(codeNotFound, http.StatusNotFound) {
		return ErrNotFound
	}
	return err
}

// Renew restarts the time to live of a sandbox. A service that does not
// support it answers unimplemented, which is not a failure for kaxa.
func (c *restClient) Renew(ctx context.Context, r Record, ttl time.Duration) error {
	err := c.call(ctx, request{
		method: http.MethodPost,
		url:    c.base + "/v1/" + r.Resource + "/renew-timeout",
		in:     map[string]string{"timeout": durationString(ttl)},
	})
	if apiErr, ok := errors.AsType[*APIError](err); ok && apiErr.is(codeUnimplemented, http.StatusNotImplemented) {
		return nil
	}
	return err
}

type credentialRequest struct {
	Permissions []string `json:"permissions"`
	TTL         string   `json:"ttl"`
}

type credentialResponse struct {
	Token string `json:"token"`
}

// Permissions of an endpoint credential that kaxa needs: run processes.
var credentialPermissions = []string{"sandboxesExec"}

// credentialTTL is the longest time to live the API grants.
const credentialTTL = 5 * time.Minute

// Credential issues a short-lived token for the process API of a sandbox.
func (c *restClient) Credential(ctx context.Context, r Record) (Credential, error) {
	var out credentialResponse
	err := c.call(ctx, request{
		method: http.MethodPost,
		url:    c.base + "/v1/" + r.Resource + "/endpoint-credentials",
		in:     credentialRequest{Permissions: credentialPermissions, TTL: durationString(credentialTTL)},
		out:    &out,
	})
	if err != nil {
		return Credential{}, err
	}
	if out.Token == "" {
		return Credential{}, errors.New("docker: the API returned no endpoint credential")
	}
	return Credential(out), nil
}

type portBody struct {
	Port struct {
		Number int `json:"number"`
	} `json:"port"`
}

type portResponse struct {
	URL string `json:"url"`
}

// PublishPort publishes a TCP port of a sandbox. The idempotency key lets a
// retry after a lost response return the first publication. A port that the
// sandbox already publishes is not a failure: its URL is read back.
func (c *restClient) PublishPort(ctx context.Context, r Record, port int) (string, error) {
	key, err := idempotencyKey()
	if err != nil {
		return "", err
	}
	var in portBody
	in.Port.Number = port
	var out portResponse
	err = c.call(ctx, request{
		method:  http.MethodPost,
		url:     c.base + "/v1/" + r.Resource + "/ports",
		headers: map[string]string{idempotencyHeader: key},
		in:      in,
		out:     &out,
	})
	if apiErr, ok := errors.AsType[*APIError](err); ok && apiErr.is(codeAlreadyExists, http.StatusConflict) {
		return c.publishedURL(ctx, r, port)
	}
	if err != nil {
		return "", err
	}
	if out.URL == "" {
		return "", errors.New("docker: the API returned no URL for the published port")
	}
	return out.URL, nil
}

// publishedURL reads the URL of a port that the sandbox already publishes.
func (c *restClient) publishedURL(ctx context.Context, r Record, port int) (string, error) {
	cur, err := c.Get(ctx, r.Resource)
	if err != nil {
		return "", err
	}
	for _, p := range cur.Ports {
		if p.Port == port && p.URL != "" {
			return p.URL, nil
		}
	}
	return "", fmt.Errorf("docker: port %d is published, but the API returned no URL for it", port)
}
