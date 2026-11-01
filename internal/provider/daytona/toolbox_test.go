package daytona

import (
	"errors"
	"strings"
	"testing"

	sdkerrors "github.com/daytona/clients/sdk-go/pkg/errors"
	"github.com/daytona/clients/sdk-go/pkg/types"
)

func TestQuoteKeepsEveryByteLiteral(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"plain", `'plain'`},
		{"two words", `'two words'`},
		{"it's", `'it'\''s'`},
		{"$(rm -rf /); `id`", "'$(rm -rf /); `id`'"},
		{"", `''`},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			expect(t, "quote", quote(tt.in), tt.want)
		})
	}
}

func TestCommandLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		spec   runSpec
		prefix string
		want   string
	}{
		{"argv only", runSpec{argv: []string{"echo", "a b"}}, "", `'echo' 'a b'`},
		{"directory", runSpec{argv: []string{"ls"}, dir: "/home/x y"}, "", `cd '/home/x y' && 'ls'`},
		{"environment", runSpec{argv: []string{"go", "test"}, env: []string{"A=1", "B=it's"}}, "", `env 'A=1' 'B=it'\''s' 'go' 'test'`},
		{"prefix", runSpec{argv: []string{"sh"}, dir: "/w"}, "exec ", `cd '/w' && exec 'sh'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			expect(t, "command line", commandLine(tt.spec, tt.prefix), tt.want)
		})
	}
}

func TestEmitWritesOnlyNewText(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	var copyErr error
	seen := emit(&out, "abc", 0, &copyErr)
	seen = emit(&out, "abc", seen, &copyErr)
	seen = emit(&out, "abcdef", seen, &copyErr)
	expect(t, "output", out.String(), "abcdef")
	expect(t, "seen", seen, 6)
	if copyErr != nil {
		t.Fatalf("copyErr = %v", copyErr)
	}
}

func TestStatusOf(t *testing.T) {
	t.Parallel()
	tests := []struct{ state, want string }{
		{"started", statusRunning},
		{"stopped", statusStopped},
		{"build_failed", "error"},
		{"destroying", "deleted"},
		{"destroyed", "deleted"},
		{"starting", "starting"},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			t.Parallel()
			expect(t, "status", statusOf(tt.state), tt.want)
		})
	}
}

func TestConvertErr(t *testing.T) {
	t.Parallel()
	if convertErr(nil) != nil {
		t.Fatal("nil error changed")
	}
	api, ok := errors.AsType[*APIError](convertErr(&sdkerrors.DaytonaError{StatusCode: 401, Message: "no"}))
	if !ok || api.Status != 401 {
		t.Fatalf("api = %+v", api)
	}
	plain := errors.New("boom")
	if !errors.Is(convertErr(plain), plain) {
		t.Fatal("plain error was wrapped")
	}
}

func TestImageOf(t *testing.T) {
	t.Parallel()
	reason := "disk full"
	tests := []struct {
		state, want string
	}{
		{"active", "active"},
		{"error", statusFailed},
		{"build_failed", statusFailed},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			t.Parallel()
			img := imageOf(&types.Snapshot{Name: "s", State: tt.state, ErrorReason: &reason})
			expect(t, "status", img.Status, tt.want)
			expect(t, "error", img.Error, reason)
		})
	}
}
