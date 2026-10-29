package cli

import (
	"errors"
	"slices"
	"testing"
)

const goosLinux = "linux"

func TestBrowserCommand(t *testing.T) {
	t.Parallel()

	const good = "https://demo.sprites.app/?a=1&b=2"
	tests := []struct {
		name     string
		goos     string
		url      string
		wantName string
		wantArgs []string
		wantErr  bool
	}{
		{name: "darwin", goos: "darwin", url: good, wantName: "open", wantArgs: []string{good}},
		{name: "linux", goos: goosLinux, url: good, wantName: "xdg-open", wantArgs: []string{good}},
		{name: "windows avoids cmd", goos: "windows", url: good, wantName: "rundll32", wantArgs: []string{"url.dll,FileProtocolHandler", good}},
		{name: "http refused", goos: goosLinux, url: "http://demo.sprites.app/", wantErr: true},
		{name: "file scheme", goos: "darwin", url: "file:///etc/passwd", wantErr: true},
		{name: "javascript scheme", goos: goosLinux, url: "javascript:alert(1)", wantErr: true},
		{name: "windows command separator", goos: "windows", url: "https://x.sprites.app/ & calc.exe", wantErr: true},
		{name: "windows pipe", goos: "windows", url: "https://x.sprites.app/|calc.exe", wantErr: true},
		{name: "option injection", goos: goosLinux, url: "--help", wantErr: true},
		{name: "userinfo", goos: goosLinux, url: "https://user:pw@x.sprites.app/", wantErr: true}, //gosec:disable G101 -- fixture: a URL with userinfo must be refused
		{name: "newline", goos: goosLinux, url: "https://x.sprites.app/\nfoo", wantErr: true},
		{name: "no host", goos: goosLinux, url: "https:///path", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			name, args, err := browserCommand(tt.goos, tt.url)
			if tt.wantErr {
				if !errors.Is(err, errUnsafeURL) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil || name != tt.wantName || !slices.Equal(args, tt.wantArgs) {
				t.Fatalf("got %q %v %v", name, args, err)
			}
		})
	}
}
