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
func Write(w io.Writer) {
	fmt.Fprintf(w, "sbx %s\n", label())
	fmt.Fprintf(w, "on %s_%s\n", runtime.GOOS, runtime.GOARCH)
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
