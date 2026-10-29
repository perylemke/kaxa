package toolchain

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/perylemke/kaxa/internal/project"
)

func init() { retryDelay = 0 }

const goAwayMsg = `go: http2: server sent GOAWAY and closed the connection; debug="server_shutting_down"`

// flakyRunner fails the project command with err the first failures times.
type flakyRunner struct {
	setupStub
	err      error
	failures int
	attempts int
}

func (f *flakyRunner) Run(ctx context.Context, dir string, env, argv []string, stdin io.Reader) (string, error) {
	if len(argv) > 1 && argv[1] == projectBuild {
		f.attempts++
		if f.attempts <= f.failures {
			return "", f.err
		}
		return "", nil
	}
	return f.setupStub.Run(ctx, dir, env, argv, stdin)
}

func TestSetupRetriesTransientNetworkErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		err          error
		failures     int
		wantAttempts int
		wantErr      bool
	}{
		{name: "recovers on the second try", err: errors.New(goAwayMsg), failures: 1, wantAttempts: 2},
		{name: "recovers on the last try", err: errors.New("read: connection reset by peer"), failures: 2, wantAttempts: 3},
		{name: "gives up after three tries", err: errors.New(goAwayMsg), failures: 5, wantAttempts: 3, wantErr: true},
		{name: "does not retry a build error", err: errors.New("./main.go:3: undefined: x"), failures: 5, wantAttempts: 1, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := &flakyRunner{err: tt.err, failures: tt.failures}
			err := Setup(context.Background(), r, testLayout, testWorkDir, []project.Step{{
				Kind: project.KindGo, Commands: [][]string{{"go", projectBuild, testAllPkgs}},
			}})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if r.attempts != tt.wantAttempts {
				t.Fatalf("attempts = %d, want %d", r.attempts, tt.wantAttempts)
			}
		})
	}
}
