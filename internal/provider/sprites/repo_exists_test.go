package sprites_test

import (
	"context"
	"errors"
	"io"
	"slices"
	"testing"

	spritesdk "github.com/superfly/sprites-go"

	"github.com/perylemke/kaxa/internal/provider/sprites"
	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestRepoExistsRunsTestDirInWorkspace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"directory present", nil, true},
		{"directory absent", &spritesdk.ExitError{Code: 1}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got []string
			client := &fakeSDKClient{
				run: func(_ context.Context, _ string, argv []string, _ io.Reader, _, _ io.Writer) error {
					got = slices.Clone(argv)
					return tt.err
				},
			}
			p := sprites.NewWithClient(client, "token")
			var _ sandbox.RepoChecker = p

			exists, err := p.RepoExists(context.Background(), "box", "api")
			if err != nil || exists != tt.want {
				t.Fatalf("RepoExists = %v, %v, want %v", exists, err, tt.want)
			}
			if want := []string{"test", "-d", "/home/sprite/workspace/api"}; !slices.Equal(got, want) {
				t.Fatalf("argv = %q, want %q", got, want)
			}
		})
	}
}

func TestRepoExistsReportsRealFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{"other exit code", &spritesdk.ExitError{Code: 2}},
		{"transport error", errors.New("connection reset")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client := &fakeSDKClient{
				run: func(context.Context, string, []string, io.Reader, io.Writer, io.Writer) error { return tt.err },
			}
			exists, err := sprites.NewWithClient(client, "token").RepoExists(context.Background(), "box", "api")
			if err == nil || exists {
				t.Fatalf("RepoExists = %v, %v, want an error", exists, err)
			}
		})
	}
}

func TestRepoExistsRejectsBadInput(t *testing.T) {
	t.Parallel()

	called := false
	client := &fakeSDKClient{
		run: func(context.Context, string, []string, io.Reader, io.Writer, io.Writer) error {
			called = true
			return nil
		},
	}
	p := sprites.NewWithClient(client, "token")
	if _, err := p.RepoExists(context.Background(), "Bad Name", "api"); err == nil {
		t.Fatal("invalid sandbox name accepted")
	}
	for _, repo := range []string{"", ".", "..", "a/b", `a\b`} {
		if _, err := p.RepoExists(context.Background(), "box", repo); err == nil {
			t.Fatalf("repo %q accepted", repo)
		}
	}
	if called {
		t.Fatal("a command ran for invalid input")
	}
}
