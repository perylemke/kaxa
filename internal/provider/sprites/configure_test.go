package sprites_test

import (
	"testing"

	"github.com/perylemke/kaxa/internal/provider/sprites"
)

type closeCountingClient struct {
	*fakeSDKClient
	closed int
}

func (c *closeCountingClient) Close() error {
	c.closed++
	return nil
}

func TestConfigureSameTokenKeepsClient(t *testing.T) {
	t.Parallel()

	client := &closeCountingClient{fakeSDKClient: &fakeSDKClient{}}
	p := sprites.NewWithClient(client, "token")

	if err := p.Configure("token"); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if client.closed != 0 {
		t.Fatalf("client closed %d times for an unchanged token", client.closed)
	}
}

func TestConfigureNewTokenReplacesClient(t *testing.T) {
	t.Parallel()

	client := &closeCountingClient{fakeSDKClient: &fakeSDKClient{}}
	p := sprites.NewWithClient(client, "token")

	if err := p.Configure("other-token"); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if client.closed != 1 {
		t.Fatalf("client closed %d times, want 1", client.closed)
	}
}
