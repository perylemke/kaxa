//go:build e2e

package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBinaryHelpAndVersion(t *testing.T) {
	bin := buildBinary(t)

	help := run(t, bin, "--help")
	if !strings.Contains(help.stdout, "sbx") {
		t.Fatalf("help missing sbx: %q", help.stdout)
	}

	version := run(t, bin, "version")
	if version.exitCode != 0 {
		t.Fatalf("version exit = %d", version.exitCode)
	}
	if !strings.Contains(version.stdout, "sbx ") {
		t.Fatalf("unexpected version: %q", version.stdout)
	}
	if !strings.Contains(version.stdout, "on ") {
		t.Fatalf("version should include platform line: %q", version.stdout)
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	out := filepath.Join(t.TempDir(), "sbx")
	cmd := exec.Command("go", "build", "-o", out, "./cmd/sbx")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
	)
	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, outBytes)
	}
	return out
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

type runResult struct {
	stdout   string
	stderr   string
	exitCode int
}

func run(t *testing.T, bin string, args ...string) runResult {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("run %v: %v", args, err)
		}
	}
	return runResult{
		stdout:   stdout.String(),
		stderr:   stderr.String(),
		exitCode: exitCode,
	}
}
