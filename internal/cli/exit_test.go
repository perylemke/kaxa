package cli

import "testing"

func TestRemoteExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code int
		want int
	}{
		{code: 0, want: 0},
		{code: 7, want: 7},
		{code: 256, want: 1},
		{code: -1, want: 1},
	}
	for _, tt := range tests {
		if got := remoteExitCode(tt.code); got != tt.want {
			t.Fatalf("remoteExitCode(%d) = %d, want %d", tt.code, got, tt.want)
		}
	}
}
