package modal_test

import (
	"context"
	"errors"
	"testing"

	"github.com/perylemke/sbox/internal/provider/modal"
	"github.com/perylemke/sbox/internal/sandbox"
)

func TestStubIsNotAvailable(t *testing.T) {
	t.Parallel()

	if sandbox.IsAvailable(modal.New()) {
		t.Fatal("modal stub should not be available")
	}
}

func TestStubMethodsNotImplemented(t *testing.T) {
	t.Parallel()

	p := modal.New()
	ctx := context.Background()

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "create",
			run: func() error {
				_, err := p.Create(ctx, sandbox.Spec{Name: "demo"})
				return err
			},
		},
		{
			name: "list",
			run: func() error {
				_, err := p.List(ctx)
				return err
			},
		},
		{
			name: "exec",
			run: func() error {
				return p.Exec(ctx, "id", sandbox.ExecOptions{Command: []string{"echo"}})
			},
		},
		{
			name: "destroy",
			run: func() error {
				return p.Destroy(ctx, "id")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.run()
			if !errors.Is(err, sandbox.ErrNotImplemented) {
				t.Fatalf("err = %v, want ErrNotImplemented", err)
			}
		})
	}
}
