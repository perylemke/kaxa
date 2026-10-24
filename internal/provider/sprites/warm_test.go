package sprites

import (
	"context"
	"errors"
	"testing"
	"time"

	spritesdk "github.com/superfly/sprites-go"
)

type warmStubClient struct {
	getCalls int
	statuses []string
}

func (s *warmStubClient) CreateSprite(_ context.Context, _ string, _ []string) (*spritesdk.Sprite, error) {
	return &spritesdk.Sprite{}, nil
}

func (s *warmStubClient) GetSprite(_ context.Context, _ string) (*spritesdk.Sprite, error) {
	s.getCalls++
	status := "cold"
	if len(s.statuses) > 0 {
		idx := s.getCalls - 1
		if idx >= len(s.statuses) {
			idx = len(s.statuses) - 1
		}
		status = s.statuses[idx]
	} else if s.getCalls >= 2 {
		status = "warm"
	}
	return &spritesdk.Sprite{Status: status}, nil
}

func (s *warmStubClient) ListEntries(context.Context, string) ([]SpriteListEntry, error) {
	return nil, nil
}

func (s *warmStubClient) DestroySprite(context.Context, string) error { return nil }

func (s *warmStubClient) Sprite(string) *spritesdk.Sprite { return nil }

func (s *warmStubClient) Close() error { return nil }

func TestWaitForWarmPollsUntilWarm(t *testing.T) {
	t.Parallel()

	client := &warmStubClient{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := waitForWarm(ctx, client, "demo", "tok"); err != nil {
		t.Fatalf("waitForWarm: %v", err)
	}
	if client.getCalls < 2 {
		t.Fatalf("getCalls = %d, want at least 2", client.getCalls)
	}
}

func TestWaitForWarmTimeout(t *testing.T) {
	t.Parallel()

	client := &warmStubClient{statuses: []string{"cold"}}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	err := waitForWarm(ctx, client, "demo", "tok")
	if !errors.Is(err, ErrWarmTimeout) {
		t.Fatalf("err = %v, want ErrWarmTimeout", err)
	}
}
