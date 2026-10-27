package sprites

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

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

func postSpriteAction(ctx context.Context, baseURL, token, path string) (err error) {
	url := strings.TrimRight(baseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
