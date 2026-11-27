package cli

import (
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

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
		{name: goosDarwin, goos: goosDarwin, url: good, wantName: "open", wantArgs: []string{good}},
		{name: goosLinux, goos: goosLinux, url: good, wantName: "xdg-open", wantArgs: []string{good}},
		{name: "windows avoids cmd", goos: goosWindows, url: good, wantName: "rundll32", wantArgs: []string{"url.dll,FileProtocolHandler", good}},
		{name: "http refused", goos: goosLinux, url: "http://demo.sprites.app/", wantErr: true},
		{name: "file scheme", goos: goosDarwin, url: "file:///etc/passwd", wantErr: true},
		{name: "javascript scheme", goos: goosLinux, url: "javascript:alert(1)", wantErr: true},
		{name: "windows command separator", goos: goosWindows, url: "https://x.sprites.app/ & calc.exe", wantErr: true},
		{name: "windows pipe", goos: goosWindows, url: "https://x.sprites.app/|calc.exe", wantErr: true},
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

func TestOpenBrowserURLInvokesLauncher(t *testing.T) {
	restore := SetBrowserExecCommandForTest(func(string, ...string) *exec.Cmd {
		// #nosec G204 -- test double for openBrowserURL; fixed argv from browserCommand
		return exec.Command("true")
	})
	t.Cleanup(restore)

	if err := openBrowserURL("https://example.com/path"); err != nil {
		t.Fatal(err)
	}
}

func TestOpenBrowserURLEmptyNoOp(t *testing.T) {
	if err := openBrowserURL(""); err != nil {
		t.Fatal(err)
	}
}

func TestLoopbackCommand(t *testing.T) {
	t.Parallel()

	const good = "http://127.0.0.1:4000/"
	tests := []struct {
		name     string
		goos     string
		url      string
		wantName string
		wantArgs []string
		wantErr  bool
	}{
		{name: "darwin", goos: goosDarwin, url: good, wantName: "open", wantArgs: []string{good}},
		{name: "linux", goos: goosLinux, url: good, wantName: "xdg-open", wantArgs: []string{good}},
		{name: "windows", goos: goosWindows, url: good, wantName: "rundll32", wantArgs: []string{"url.dll,FileProtocolHandler", good}},
		{name: "https refused", goos: goosLinux, url: "https://127.0.0.1:4000/", wantErr: true},
		{name: "other host", goos: goosLinux, url: "http://localhost:4000/", wantErr: true},
		{name: "query refused", goos: goosLinux, url: "http://127.0.0.1:4000/?token=1", wantErr: true},
		{name: "missing port", goos: goosLinux, url: "http://127.0.0.1/", wantErr: true},
		{name: "path refused", goos: goosLinux, url: "http://127.0.0.1:4000/admin", wantErr: true},
		{name: "userinfo", goos: goosLinux, url: "http://user:pw@127.0.0.1:4000/", wantErr: true}, //gosec:disable G101 -- fixture: a URL with userinfo must be refused
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			name, args, err := loopbackCommand(tt.goos, tt.url)
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

func TestOpenLoopbackURLInvokesLauncher(t *testing.T) {
	restore := SetBrowserExecCommandForTest(func(string, ...string) *exec.Cmd {
		// #nosec G204 -- test double for openLoopbackURL; fixed argv from loopbackCommand
		return exec.Command("true")
	})
	t.Cleanup(restore)

	if err := openLoopbackURL("http://127.0.0.1:4000/"); err != nil {
		t.Fatal(err)
	}
}

func TestOpenLoopbackURLEmptyNoOp(t *testing.T) {
	if err := openLoopbackURL(""); err != nil {
		t.Fatal(err)
	}
}

func TestOpenLoopbackURLRejectsAndReportsLauncherErrors(t *testing.T) {
	if err := openLoopbackURL("http://127.0.0.1:9/\n"); !errors.Is(err, errUnsafeURL) {
		t.Fatalf("newline err = %v", err)
	}
	if err := checkLoopbackURL(`http://127.0.0.1:9/"`); !errors.Is(err, errUnsafeURL) {
		t.Fatalf("quote err = %v", err)
	}

	restore := SetBrowserExecCommandForTest(func(string, ...string) *exec.Cmd {
		return exec.Command("false")
	})
	t.Cleanup(restore)
	err := openLoopbackURL("http://127.0.0.1:9/")
	if err == nil || !strings.Contains(err.Error(), "open browser") {
		t.Fatalf("launcher err = %v", err)
	}
}

func TestOpenBrowserURLRefusesUnsafeURLs(t *testing.T) {
	if err := openBrowserURL("http://example.com/"); !errors.Is(err, errUnsafeURL) {
		t.Fatalf("err = %v", err)
	}
}

func TestOpenBrowserURLReportsLauncherFailure(t *testing.T) {
	restore := SetBrowserExecCommandForTest(func(string, ...string) *exec.Cmd {
		// #nosec G204 -- test double: a launcher that exits with an error
		return exec.Command("false")
	})
	t.Cleanup(restore)

	err := openBrowserURL("https://example.com/path")
	if err == nil || !strings.Contains(err.Error(), "open browser") {
		t.Fatalf("err = %v", err)
	}
}
