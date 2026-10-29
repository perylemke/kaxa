package cli

import (
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"unicode"
)

var openBrowserFn = openBrowserURL

var errUnsafeURL = errors.New("refusing to open a URL that is not a plain https address")

func openBrowserURL(rawURL string) error {
	if rawURL == "" {
		return nil
	}
	name, args, err := browserCommand(runtime.GOOS, rawURL)
	if err != nil {
		return err
	}
	cmd := exec.Command(name, args...) // #nosec G204 -- launcher is a fixed OS binary; the URL is checked by browserCommand
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
	switch goos {
	case "darwin":
		return "open", []string{rawURL}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", rawURL}, nil
	default:
		return "xdg-open", []string{rawURL}, nil
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
