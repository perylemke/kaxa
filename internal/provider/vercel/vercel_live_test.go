//go:build live

package vercel

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// TestLiveLifecycle talks to the real Vercel API and bills compute. It is
// skipped without credentials and never runs in make ci.
func TestLiveLifecycle(t *testing.T) {
	token, project := os.Getenv("KAXA_VERCEL_TOKEN"), os.Getenv("KAXA_VERCEL_PROJECT")
	if token == "" || project == "" {
		t.Skip("set KAXA_VERCEL_TOKEN and KAXA_VERCEL_PROJECT to run")
	}
	p := New()
	if err := p.Configure(token, project, os.Getenv("KAXA_VERCEL_TEAM")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	name := "kaxa-live-" + time.Now().Format("150405")
	id, err := p.Create(ctx, sandbox.Spec{Name: name, Labels: []string{"kaxa"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Destroy(context.WithoutCancel(ctx), id) })

	// The CLI calls PrepareWorkspace after every create. It also creates
	// /vercel/sandbox, which does not exist in a fresh sandbox.
	if err := p.PrepareWorkspace(ctx, id); err != nil {
		t.Fatal(err)
	}

	// These lines confirm the facts the provider assumes: the home directory,
	// the user, and that a manual snapshot can be restored in place.
	var out bytes.Buffer
	err = p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{"sh", "-c", "pwd; id -un; echo $HOME"}, Stdout: &out})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("pwd, user, HOME:\n%s", out.String())
	if !strings.Contains(out.String(), vercelHome) {
		t.Errorf("working directory is not %s", vercelHome)
	}
	snap, err := p.Snapshot(ctx, id, "live")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Restore(ctx, sandbox.RestoreRequest{Sandbox: id, Snapshot: snap.ID}); err != nil {
		t.Fatal(err)
	}
	if err := p.DeleteSnapshot(ctx, id, snap.ID); err != nil {
		t.Fatal(err)
	}
}

// TestLiveConsole opens the interactive terminal of a real sandbox, runs one
// command in it, and checks the output and the exit code.
func TestLiveConsole(t *testing.T) {
	token, project := os.Getenv("KAXA_VERCEL_TOKEN"), os.Getenv("KAXA_VERCEL_PROJECT")
	if token == "" || project == "" {
		t.Skip("set KAXA_VERCEL_TOKEN and KAXA_VERCEL_PROJECT to run")
	}
	p := New()
	if err := p.Configure(token, project, os.Getenv("KAXA_VERCEL_TEAM")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	id, err := p.Create(ctx, sandbox.Spec{Name: "kaxa-live-pty-" + time.Now().Format("150405"), Labels: []string{"kaxa"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Destroy(context.WithoutCancel(ctx), id) })
	if err := p.PrepareWorkspace(ctx, id); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err = p.Exec(ctx, id, sandbox.ExecOptions{
		Command: []string{"bash"}, TTY: true, Stdout: &out,
		Stdin: strings.NewReader("echo kaxa-console-ok; exit 4\n"),
		Size:  sandbox.TermSize{Rows: 24, Cols: 80},
	})
	var exit *sandbox.ExitError
	if !errors.As(err, &exit) || exit.Code != 4 {
		t.Fatalf("Exec() = %v, want exit status 4", err)
	}
	if !strings.Contains(out.String(), "kaxa-console-ok") {
		t.Fatalf("terminal output = %q", out.String())
	}
}
