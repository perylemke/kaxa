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
	warmGetTimeout   = 10 * time.Second
	warmTimeout      = 2 * time.Minute
)

// ErrWarmTimeout means the sprite was created but did not become ready in time.
var ErrWarmTimeout = errors.New("sprite did not become warm before timeout")

// spriteReady reports a sprite that can run a command. running is the active
// state; warm is idle and wakes on the next command.
func spriteReady(status string) bool {
	return strings.EqualFold(status, "warm") || strings.EqualFold(status, "running")
}

func waitForWarm(ctx context.Context, c sdkClient, name, token string) error {
	waitCtx, cancel := context.WithTimeout(ctx, warmTimeout)
	defer cancel()

	ticker := time.NewTicker(warmPollInterval)
	defer ticker.Stop()

	var status string
	for {
		pollCtx, pollCancel := context.WithTimeout(waitCtx, warmGetTimeout)
		sp, err := c.GetSprite(pollCtx, name)
		pollCancel()
		if err != nil {
			if waitCtx.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
				if waitErr := waitWarmPoll(waitCtx, ticker); waitErr != nil {
					return warmTimeoutError(name, status, waitErr)
				}
				continue
			}
			if waitCtx.Err() != nil {
				return warmTimeoutError(name, status, waitCtx.Err())
			}
			return redactErr(err, token)
		}
		status = sp.Status
		if spriteReady(status) {
			return nil
		}
		if waitErr := waitWarmPoll(waitCtx, ticker); waitErr != nil {
			return warmTimeoutError(name, status, waitErr)
		}
	}
}

func waitWarmPoll(ctx context.Context, ticker *time.Ticker) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ticker.C:
		return nil
	}
}

func warmTimeoutError(name, status string, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		msg := fmt.Sprintf("sandbox %q was created but is still %q", name, status)
		return usererr.Wrap(ErrWarmTimeout, msg, "check kaxa list, or destroy it and create again")
	}
	return err
}
