package sprites

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	spritesdk "github.com/superfly/sprites-go"
)

func TestPublishAPIFailedStatus(t *testing.T) {
	p := providerWithAPIStates(t, apiStateJSON(t, apiStatusFailed, "exited with code 1"))
	start := time.Now()
	err := p.PublishAPI(context.Background(), "box", spriteHome, 8000, []string{"uvicorn"}, nil)
	if time.Since(start) >= 500*time.Millisecond {
		t.Fatalf("waited %s", time.Since(start))
	}
	if err == nil || !strings.Contains(err.Error(), "exited with code 1") {
		t.Fatalf("err = %v", err)
	}
}

func TestPublishAPIBecomesRunning(t *testing.T) {
	setAPIReadyForTest(t, time.Second, 5*time.Millisecond)
	p := providerWithAPIStates(t,
		apiStateJSON(t, "starting", ""),
		apiStateJSON(t, apiStatusRunning, ""),
	)
	if err := p.PublishAPI(context.Background(), "box", spriteHome, 8000, []string{"uvicorn"}, nil); err != nil {
		t.Fatalf("PublishAPI: %v", err)
	}
}

func TestPublishAPIReadyTimeout(t *testing.T) {
	setAPIReadyForTest(t, 40*time.Millisecond, 5*time.Millisecond)
	p := providerWithAPIStates(t, apiStateJSON(t, "starting", ""))
	err := p.PublishAPI(context.Background(), "box", spriteHome, 8000, []string{"uvicorn"}, nil)
	if err == nil || !strings.Contains(err.Error(), "did not become running") {
		t.Fatalf("err = %v", err)
	}
}

func TestPublishAPIWaitsAtLeastNinetySeconds(t *testing.T) {
	if apiReadyTimeout != 90*time.Second {
		t.Fatalf("apiReadyTimeout = %s", apiReadyTimeout)
	}
}

func setAPIReadyForTest(t *testing.T, timeout, interval time.Duration) {
	t.Helper()
	prevTimeout, prevInterval := apiReadyTimeout, apiReadyInterval
	apiReadyTimeout = timeout
	apiReadyInterval = interval
	t.Cleanup(func() {
		apiReadyTimeout = prevTimeout
		apiReadyInterval = prevInterval
	})
}

func providerWithAPIStates(t *testing.T, states ...string) *Provider {
	t.Helper()
	return NewWithClient(&readyClient{states: states}, "token")
}

func apiStateJSON(t *testing.T, status, detail string) string {
	t.Helper()
	body, err := json.Marshal(map[string]map[string]string{
		"state": {"status": status, "error": detail},
	})
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	return string(body)
}

type readyClient struct {
	states []string
	gets   int
}

func (c *readyClient) RunIn(_ context.Context, _, _ string, _, argv []string, _ io.Reader, stdout, _ io.Writer) error {
	if len(argv) < 3 || argv[1] != servicesCmd || argv[2] != "get" {
		return nil
	}
	state := c.states[len(c.states)-1]
	if c.gets < len(c.states) {
		state = c.states[c.gets]
	}
	c.gets++
	_, err := io.WriteString(stdout, state)
	return err
}

func (c *readyClient) Run(ctx context.Context, name string, argv []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return c.RunIn(ctx, name, "", nil, argv, stdin, stdout, stderr)
}

func (c *readyClient) CreateSprite(context.Context, string, []string) (*spritesdk.Sprite, error) {
	return &spritesdk.Sprite{}, nil
}

func (c *readyClient) GetSprite(context.Context, string) (*spritesdk.Sprite, error) {
	return &spritesdk.Sprite{}, nil
}

func (c *readyClient) ListEntries(context.Context, string) ([]SpriteListEntry, error) {
	return nil, nil
}

func (c *readyClient) UpgradeSprite(context.Context, string) error { return nil }

func (c *readyClient) RestartSprite(context.Context, string) error { return nil }

func (c *readyClient) DestroySprite(context.Context, string) error { return nil }

func (c *readyClient) Sprite(string) *spritesdk.Sprite { return &spritesdk.Sprite{} }

func (c *readyClient) Close() error { return nil }
