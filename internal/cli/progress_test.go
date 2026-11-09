package cli

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestRunTaskWithSpinnerNonTTYRunsTaskDirectly(t *testing.T) {
	t.Parallel()

	var ran bool
	err := runTaskWithSpinner(context.Background(), new(bytes.Buffer), "x", func(context.Context) error {
		ran = true
		return errors.New("boom")
	})
	if !ran || err == nil || err.Error() != "boom" {
		t.Fatalf("ran=%v err=%v", ran, err)
	}
}
