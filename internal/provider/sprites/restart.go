package sprites

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// Restart reboots a sprite.
func (p *Provider) Restart(ctx context.Context, id sandbox.ID) error {
	if err := validateName(string(id)); err != nil {
		return err
	}
	c, token, err := p.clientAndToken()
	if err != nil {
		return err
	}
	if err := c.RestartSprite(ctx, string(id)); err != nil {
		return redactErr(err, token)
	}
	return nil
}

// statusError is a non-2xx answer from the Sprites API.
type statusError struct {
	Code int
	Body string
}

func (e *statusError) Error() string {
	return fmt.Sprintf("API returned status %d: %s", e.Code, e.Body)
}

const (
	// apiTimeout bounds one call to the Sprites HTTP API.
	apiTimeout = 30 * time.Second
	// maxErrorBody caps how much of an error response is read and shown.
	maxErrorBody = 4 << 10
)

// apiHTTPClient is the client for the Sprites HTTP routes the SDK does not cover.
var apiHTTPClient = &http.Client{Timeout: apiTimeout}

// errorBody reads at most maxErrorBody bytes of a response, for an error message.
func errorBody(resp *http.Response) string {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	return strings.TrimSpace(strings.ToValidUTF8(string(body), "\uFFFD"))
}

func postSpriteAction(ctx context.Context, baseURL, token, path string) error {
	return spriteRequest(ctx, http.MethodPost, baseURL, token, path)
}

func spriteRequest(ctx context.Context, method, baseURL, token, path string) (err error) {
	url := strings.TrimRight(baseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := apiHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= 300 {
		return &statusError{Code: resp.StatusCode, Body: errorBody(resp)}
	}
	return nil
}
