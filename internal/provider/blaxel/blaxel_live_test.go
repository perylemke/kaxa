//go:build live

package blaxel_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/provider/blaxel"
	"github.com/perylemke/kaxa/internal/sandbox"
)

// liveProvider returns a configured provider, or skips the test without
// KAXA_BLAXEL_API_KEY and KAXA_BLAXEL_WORKSPACE. These tests bill Blaxel compute.
func liveProvider(t *testing.T) *blaxel.Provider {
	t.Helper()
	key := os.Getenv("KAXA_BLAXEL_API_KEY")
	workspace := os.Getenv("KAXA_BLAXEL_WORKSPACE")
	if key == "" || workspace == "" {
		t.Skip("KAXA_BLAXEL_API_KEY or KAXA_BLAXEL_WORKSPACE is not set")
	}
	p := blaxel.New()
	if err := p.Configure(key, workspace); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if err := p.SetDefaultTemplate(os.Getenv("KAXA_BLAXEL_TEMPLATE")); err != nil {
		t.Fatalf("template: %v", err)
	}
	if err := p.SetRegion(os.Getenv("KAXA_BLAXEL_REGION")); err != nil {
		t.Fatalf("region: %v", err)
	}
	return p
}

func TestRealBlaxelExec(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	name := "kaxa-live-" + randomSuffix(t)
	id, err := p.Create(ctx, sandbox.Spec{Name: name, Labels: []string{"kaxa"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 2*time.Minute)
		defer stop()
		_ = p.Destroy(cleanup, id)
	})

	var user strings.Builder
	if err := p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{"id", "-un"}, Stdout: &user}); err != nil {
		t.Fatalf("id: %v", err)
	}
	if strings.TrimSpace(user.String()) != "root" {
		t.Fatalf("user = %q, want root", user.String())
	}

	arg := "a b'$(echo pwned);echo"
	var out strings.Builder
	if err := p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{"printf", "%s", arg}, Stdout: &out}); err != nil {
		t.Fatalf("printf: %v", err)
	}
	if out.String() != arg {
		t.Fatalf("printf = %q", out.String())
	}
}

func TestRealBlaxelConsole(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	name := "kaxa-live-" + randomSuffix(t)
	id, err := p.Create(ctx, sandbox.Spec{Name: name, Labels: []string{"kaxa"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 2*time.Minute)
		defer stop()
		_ = p.Destroy(cleanup, id)
	})

	shell, stop := context.WithTimeout(ctx, 2*time.Minute)
	defer stop()
	var out strings.Builder
	err = p.Exec(shell, id, sandbox.ExecOptions{
		Command: []string{"bash"}, TTY: true,
		Stdin:  strings.NewReader("id -un\nexit\n"),
		Stdout: &out, Stderr: &out,
		Size: sandbox.TermSize{Rows: 24, Cols: 80},
	})
	if err != nil {
		t.Fatalf("console: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "root") {
		t.Fatalf("console = %q", out.String())
	}
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(b[:])
}
