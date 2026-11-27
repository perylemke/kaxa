package cli

import (
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"unicode"
)

const (
	goosDarwin  = "darwin"
	goosLinux   = "linux"
	goosWindows = "windows"
)

var openBrowserFn = openBrowserURL

// browserExecCommand builds the launcher process; tests replace it.
var browserExecCommand = exec.Command // #nosec G204 -- launcher is a fixed OS binary; the URL is checked before it is passed

var errUnsafeURL = errors.New("refusing to open a URL that is not a plain https address")

func openBrowserURL(rawURL string) error {
	if rawURL == "" {
		return nil
	}
	name, args, err := browserCommand(runtime.GOOS, rawURL)
	if err != nil {
		return err
	}
	cmd := browserExecCommand(name, args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}

// browserCommand picks the launcher for goos. The address comes from a provider
// API, so only a plain https URL is opened. On Windows the URL goes to
// rundll32, not to cmd.exe, which would read & and | in it as commands.
func browserCommand(goos, rawURL string) (string, []string, error) {
	if err := checkBrowserURL(rawURL); err != nil {
		return "", nil, err
	}
	name, args := launchCommand(goos, rawURL)
	return name, args, nil
}

// openLoopbackURL opens the local kaxa ui address. Only http://127.0.0.1:<port>/ is allowed.
func openLoopbackURL(rawURL string) error {
	if rawURL == "" {
		return nil
	}
	name, args, err := loopbackCommand(runtime.GOOS, rawURL)
	if err != nil {
		return err
	}
	cmd := browserExecCommand(name, args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}

func loopbackCommand(goos, rawURL string) (string, []string, error) {
	if err := checkLoopbackURL(rawURL); err != nil {
		return "", nil, err
	}
	name, args := launchCommand(goos, rawURL)
	return name, args, nil
}

func launchCommand(goos, rawURL string) (string, []string) {
	switch goos {
	case goosDarwin:
		return "open", []string{rawURL}
	case goosWindows:
		return "rundll32", []string{"url.dll,FileProtocolHandler", rawURL}
	default:
		return "xdg-open", []string{rawURL}
	}
}

func checkBrowserURL(rawURL string) error {
	if strings.ContainsFunc(rawURL, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) {
		return errUnsafeURL
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || strings.ContainsAny(rawURL, `"<>^|`) {
		return errUnsafeURL
	}
	return nil
}

// checkLoopbackURL allows only the address kaxa ui prints: http://127.0.0.1:<port>/.
func checkLoopbackURL(rawURL string) error {
	if loopbackTextUnsafe(rawURL) {
		return errUnsafeURL
	}
	u, err := url.Parse(rawURL)
	if err != nil || !loopbackURL(u) {
		return errUnsafeURL
	}
	return nil
}

func loopbackTextUnsafe(rawURL string) bool {
	if strings.ContainsFunc(rawURL, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) {
		return true
	}
	return strings.ContainsAny(rawURL, `"<>^|`)
}

func loopbackURL(u *url.URL) bool {
	if u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if u.Path != "" && u.Path != "/" {
		return false
	}
	port, err := strconv.Atoi(u.Port())
	return err == nil && port >= 1 && port <= 65535 && u.Host == "127.0.0.1:"+u.Port()
}
