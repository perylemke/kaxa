package sprites_test

import (
	"context"
	"errors"
	"testing"

	"github.com/perylemke/sbx/internal/provider/sprites"
	"github.com/perylemke/sbx/internal/sandbox"
)

func TestStubMethodsNotImplemented(t *testing.T) {
	t.Parallel()

	p := sprites.New()
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
				return p.Exec(ctx, "id", []string{"echo"})
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
