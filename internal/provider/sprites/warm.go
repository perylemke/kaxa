package sprites

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	warmPollInterval = time.Second
	warmTimeout      = 2 * time.Minute
)

// ErrWarmTimeout means the sprite was created but did not reach warm status in time.
var ErrWarmTimeout = errors.New("sprite did not become warm before timeout")

func waitForWarm(ctx context.Context, c sdkClient, name, token string) error {
	waitCtx, cancel := context.WithTimeout(ctx, warmTimeout)
	defer cancel()

	ticker := time.NewTicker(warmPollInterval)
	defer ticker.Stop()

	for {
		sp, err := c.GetSprite(waitCtx, name)
		if err != nil {
			return redactErr(err, token)
		}
		if strings.EqualFold(sp.Status, "warm") {
			return nil
		}

		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				msg := fmt.Sprintf("sandbox %q was created but is still %q", name, sp.Status)
				return usererr.Wrap(ErrWarmTimeout, msg, "check kaxa list, or destroy it and create again")
			}
			return waitCtx.Err()
		case <-ticker.C:
		}
	}
}
