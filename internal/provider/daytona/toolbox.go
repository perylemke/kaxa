package daytona

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	kindExec = "exec"
	kindPTY  = "pty"

	reasonExited   = "exited"
	reasonSignaled = "signaled"
	reasonTimedOut = "timed_out"

	toolboxTimeout = 2 * time.Minute
	logPageLimit   = 1000
)

// Conn locates the toolbox of one sandbox.
type Conn struct {
	// SandboxID is the ID Daytona generated.
	SandboxID string
	// BaseURL is the toolbox proxy of the sandbox region. Empty means the default.
	BaseURL string
}

// ProcessRequest describes a process to start in a sandbox.
type ProcessRequest struct {
	Kind  string
	Argv  []string
	Dir   string
	Stdin bool
	// Rows and Cols size the terminal of a pty process.
	Rows, Cols uint16
}

// LogFrame is a piece of output of a process.
type LogFrame struct {
	// Channel is stdout, stderr, or another channel such as setup.
	Channel string
	Data    string
	Cursor  string
}

// WaitResult is how a process ended, or that the wait window elapsed.
type WaitResult struct {
	Reason   string
	ExitCode int
	HasCode  bool
	Signal   string
}

// Done reports whether the process finished, as opposed to the wait timing out.
func (w WaitResult) Done() bool { return w.Reason != reasonTimedOut && w.Reason != "" }

// Attachment is the live terminal stream of a pty process.
type Attachment interface {
	// Read returns the next output, or io.EOF when the stream closes.
	Read() ([]byte, error)
	Write(data []byte) error
	Close() error
}

type toolboxClient interface {
	Start(ctx context.Context, c Conn, r ProcessRequest) (string, error)
	Logs(ctx context.Context, c Conn, id, cursor string) ([]LogFrame, error)
	// Wait blocks up to timeout for the process to finish. A zero timeout waits
	// until it does.
	Wait(ctx context.Context, c Conn, id string, timeout time.Duration) (WaitResult, error)
	Stdin(ctx context.Context, c Conn, id string, data []byte, eof bool) error
	Resize(ctx context.Context, c Conn, id string, rows, cols uint16) error
	Attach(ctx context.Context, c Conn, id string) (Attachment, error)
	Upload(ctx context.Context, c Conn, path string, r io.Reader) error
}

type httpToolbox struct {
	key  string
	http *http.Client
}

func newHTTPToolbox(apiKey string) *httpToolbox {
	return &httpToolbox{key: apiKey, http: &http.Client{}}
}

// processBase returns <toolbox>/toolbox/<sandbox>, after checking that the
// toolbox URL the API gave is HTTPS: the API key goes to it.
func processBase(c Conn) (string, error) {
	raw := c.BaseURL
	if raw == "" {
		raw = defaultToolboxURL
	}
	u, err := checkEndpoint(raw)
	if err != nil {
		return "", err
	}
	if c.SandboxID == "" || strings.ContainsAny(c.SandboxID, `/\?#`) {
		return "", errors.New("daytona: invalid sandbox ID")
	}
	return strings.TrimRight(u.String(), "/") + "/toolbox/" + url.PathEscape(c.SandboxID), nil
}

type startBody struct {
	Kind     string        `json:"kind"`
	Argv     []string      `json:"argv"`
	Cwd      string        `json:"cwd,omitempty"`
	Stdin    string        `json:"stdin"`
	Terminal *terminalBody `json:"terminal,omitempty"`
}

type terminalBody struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

func (t *httpToolbox) Start(ctx context.Context, c Conn, r ProcessRequest) (string, error) {
	body := startBody{Kind: r.Kind, Argv: r.Argv, Cwd: r.Dir, Stdin: "closed"}
	if r.Stdin {
		body.Stdin = "open"
	}
	if r.Kind == kindPTY {
		body.Terminal = &terminalBody{Cols: r.Cols, Rows: r.Rows}
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := t.do(ctx, c, http.MethodPost, "/processes", nil, body, &out); err != nil {
		return "", err
	}
	if out.ID == "" {
		return "", errors.New("daytona: the toolbox returned no process ID")
	}
	return out.ID, nil
}

func (t *httpToolbox) Logs(ctx context.Context, c Conn, id, cursor string) ([]LogFrame, error) {
	q := url.Values{"limit": {strconv.Itoa(logPageLimit)}}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	var page struct {
		Items []struct {
			Channel string `json:"channel"`
			Stream  string `json:"stream"`
			Data    string `json:"data"`
			Cursor  string `json:"cursor"`
		} `json:"items"`
	}
	if err := t.do(ctx, c, http.MethodGet, "/processes/"+url.PathEscape(id)+"/logs", q, nil, &page); err != nil {
		return nil, err
	}
	frames := make([]LogFrame, 0, len(page.Items))
	for _, it := range page.Items {
		ch := it.Channel
		if ch == "" {
			ch = it.Stream
		}
		frames = append(frames, LogFrame{Channel: ch, Data: it.Data, Cursor: it.Cursor})
	}
	return frames, nil
}

func (t *httpToolbox) Wait(ctx context.Context, c Conn, id string, timeout time.Duration) (WaitResult, error) {
	q := url.Values{"timeoutMs": {strconv.FormatInt(timeout.Milliseconds(), 10)}}
	var out struct {
		Reason   string `json:"reason"`
		ExitCode *int   `json:"exitCode"`
		Signal   string `json:"signal"`
	}
	if err := t.do(ctx, c, http.MethodGet, "/processes/"+url.PathEscape(id)+"/wait", q, nil, &out); err != nil {
		return WaitResult{}, err
	}
	res := WaitResult{Reason: out.Reason, Signal: out.Signal}
	if out.ExitCode != nil {
		res.ExitCode, res.HasCode = *out.ExitCode, true
	}
	return res, nil
}

func (t *httpToolbox) Stdin(ctx context.Context, c Conn, id string, data []byte, eof bool) error {
	body := map[string]any{"data": string(data), "eof": eof}
	return t.do(ctx, c, http.MethodPost, "/processes/"+url.PathEscape(id)+"/stdin", nil, body, nil)
}

func (t *httpToolbox) Resize(ctx context.Context, c Conn, id string, rows, cols uint16) error {
	body := map[string]uint16{"cols": cols, "rows": rows}
	return t.do(ctx, c, http.MethodPost, "/processes/"+url.PathEscape(id)+"/resize", nil, body, nil)
}

func (t *httpToolbox) Upload(ctx context.Context, c Conn, path string, r io.Reader) error {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "upload")
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, r); err != nil {
		return fmt.Errorf("read upload: %w", err)
	}
	if err := mw.Close(); err != nil {
		return err
	}
	return t.send(ctx, c, http.MethodPost, "/files/upload", url.Values{"path": {path}}, mw.FormDataContentType(), &buf, nil)
}

func (t *httpToolbox) do(ctx context.Context, c Conn, method, path string, query url.Values, body, out any) error {
	if body == nil {
		return t.send(ctx, c, method, path, query, "", nil, out)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return t.send(ctx, c, method, path, query, "application/json", bytes.NewReader(raw), out)
}

func (t *httpToolbox) send(ctx context.Context, c Conn, method, path string, query url.Values, contentType string, body io.Reader, out any) error {
	base, err := processBase(c)
	if err != nil {
		return err
	}
	endpoint := base + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	// A wait can park the request for as long as the command runs, so only the
	// caller's context bounds it; everything else gets a default limit.
	if !strings.HasSuffix(path, "/wait") {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, toolboxTimeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+t.key)
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := t.http.Do(req)
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
