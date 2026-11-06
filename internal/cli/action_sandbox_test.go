package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunConfirmedSandboxActionCancel(t *testing.T) {
	t.Parallel()

	out := new(bytes.Buffer)
	err := runConfirmedSandboxAction(
		context.Background(),
		strings.NewReader("n\n"),
		new(bytes.Buffer),
		out,
		"Proceed?",
		"working…",
		"done",
		"ok",
		func(context.Context) error { return errors.New("should not run") },
	)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out.String(), "cancelled") {
		t.Fatalf("out = %q", out.String())
	}
}

func TestRunConfirmedSandboxActionRunsSpinner(t *testing.T) {
	t.Parallel()

	var ran bool
	err := runConfirmedSandboxAction(
		context.Background(),
		strings.NewReader("y\n"),
		new(bytes.Buffer),
		new(bytes.Buffer),
		"Proceed?",
		"working…",
		"done",
		"ok",
		func(context.Context) error {
			ran = true
			return nil
		},
	)
	if err != nil || !ran {
		t.Fatalf("err = %v ran = %v", err, ran)
	}
}
