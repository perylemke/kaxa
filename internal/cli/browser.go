package cli

import (
	"fmt"
	"os/exec"
	"runtime"
)

var openBrowserFn = openBrowserURL

func openBrowserURL(url string) error {
	if url == "" {
		return nil
	}
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name = "open"
		args = []string{url}
	case "windows":
		name = "cmd"
		args = []string{"/c", "start", url}
	default:
		name = "xdg-open"
		args = []string{url}
	}
	cmd := exec.Command(name, args...) // #nosec G204 -- launcher is a fixed OS binary; only the URL varies
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}
