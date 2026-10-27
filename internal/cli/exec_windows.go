//go:build windows

package cli

import (
	"context"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func startWinchWatcher(_ context.Context, _ int, _ chan<- sandbox.TermSize) {}
