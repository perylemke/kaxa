package daytona

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	handshakeTimeout = 30 * time.Second
	maxFrame         = 1 << 20
)

// Attach opens the terminal stream of a pty process. Binary frames carry the
// output; what is written goes to the standard input of the process.
func (t *httpToolbox) Attach(ctx context.Context, c Conn, id string) (Attachment, error) {
	base, err := processBase(c)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(base + "/processes/" + url.PathEscape(id) + "/attach")
	if err != nil {
		return nil, err
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	dialer := websocket.Dialer{HandshakeTimeout: handshakeTimeout, Proxy: http.ProxyFromEnvironment}
	header := http.Header{"Authorization": {"Bearer " + t.key}}
	conn, resp, err := dialer.DialContext(ctx, u.String(), header) //nolint:bodyclose // closed below when the handshake fails
	if err != nil {
		if resp != nil {
			defer func() { _ = resp.Body.Close() }()
			return nil, &APIError{Status: resp.StatusCode}
		}
		return nil, fmt.Errorf("daytona: attach to process: %w", urlErrorCause(err))
	}
	conn.SetReadLimit(maxFrame)
	return &wsAttachment{conn: conn}, nil
}

// urlErrorCause drops the URL of a transport error: it names the sandbox host.
func urlErrorCause(err error) error {
	if ue, ok := errors.AsType[*url.Error](err); ok {
		return ue.Err
	}
	return err
}

// wsAttachment is an Attachment over a WebSocket. gorilla/websocket allows one
// writer at a time, so writes take a lock; one goroutine reads.
type wsAttachment struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (a *wsAttachment) Read() ([]byte, error) {
	for {
		typ, data, err := a.conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
				return nil, io.EOF
			}
			return nil, fmt.Errorf("daytona: read from process: %w", err)
		}
		if typ == websocket.BinaryMessage || typ == websocket.TextMessage {
			return data, nil
		}
	}
}

func (a *wsAttachment) Write(data []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return fmt.Errorf("daytona: write to process: %w", err)
	}
	return nil
}

func (a *wsAttachment) Close() error {
	return a.conn.Close()
}
