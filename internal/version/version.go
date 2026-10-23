// Package version holds release metadata injected at build time.
package version

import (
	"fmt"
	"io"
	"runtime"
	"strings"
)

// Set via -ldflags at build time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String returns detailed build metadata.
func String() string {
	return fmt.Sprintf("%s (commit %s, built %s)", Version, Commit, Date)
}

// Write prints version information in a Terraform-like format to w.
func Write(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "sbx %s\n", label()); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "on %s_%s\n", runtime.GOOS, runtime.GOARCH)
	return err
}

func label() string {
	if Version == "dev" {
		return "dev"
	}
	if strings.HasPrefix(Version, "v") {
		return Version
	}
	return "v" + Version
}
