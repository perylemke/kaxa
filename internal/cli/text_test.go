package cli

import "testing"

func TestPlainText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "my-box", want: "my-box"},
		{name: "keeps newline and unicode", in: "a\nb ✓", want: "a\nb ✓"},
		{name: "color escape", in: "\x1b[31mred\x1b[0m", want: "[31mred[0m"},
		{name: "title escape", in: "ok\x1b]0;pwned\x07", want: "ok]0;pwned"},
		{name: "carriage return overwrite", in: "safe\rEVIL", want: "safeEVIL"},
		{name: "c1 control", in: "a\u009bb", want: "ab"},
		{name: "line separator", in: "a\u2028b", want: "ab"},
		{name: "backspace and nul", in: "a\b\x00b", want: "ab"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := plainText(tt.in); got != tt.want {
				t.Fatalf("plainText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestJoinListLabelsIsPlain(t *testing.T) {
	t.Parallel()

	if got := joinListLabels([]string{"kaxa", "\x1b[2Jwipe"}); got != "kaxa,[2Jwipe" {
		t.Fatalf("got %q", got)
	}
}
