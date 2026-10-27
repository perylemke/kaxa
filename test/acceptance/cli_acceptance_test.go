//go:build acceptance

package acceptance_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpMentionsProviders(t *testing.T) {
	bin := buildBinary(t)
	res := run(t, bin, "--help")
	combined := res.stdout + res.stderr
	if !strings.Contains(combined, "sprites") || !strings.Contains(combined, "modal") {
		t.Fatalf("help should mention providers: %q", combined)
	}
}

func TestUnknownProviderNonZeroExit(t *testing.T) {
	bin := buildBinary(t)
	const secret = "acceptance-test-secret-token"

	res := runWithEnv(t, bin, map[string]string{
		"KAXA_SPRITES_TOKEN": secret,
	}, "--provider", "unknown")
	if res.exitCode == 0 {
		t.Fatal("expected non-zero exit for unknown provider")
	}
	if strings.Contains(res.stderr, secret) {
		t.Fatal("stderr must not contain config secret")
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	out := filepath.Join(t.TempDir(), "kaxa")
	cmd := exec.Command("go", "build", "-o", out, "./cmd/kaxa")
	cmd.Dir = root
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

func runWithEnv(t *testing.T, bin string, env map[string]string, args ...string) runResult {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
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
