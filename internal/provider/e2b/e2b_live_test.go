//go:build live

package e2b_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/provider/e2b"
	"github.com/perylemke/kaxa/internal/sandbox"
)

// liveProvider returns a configured provider, or skips the test without
// KAXA_E2B_API_KEY. These tests bill a few minutes of E2B compute.
func liveProvider(t *testing.T) *e2b.Provider {
	t.Helper()
	key := os.Getenv("KAXA_E2B_API_KEY")
	if key == "" {
		t.Skip("KAXA_E2B_API_KEY is not set")
	}
	p := e2b.New()
	if err := p.Configure(key); err != nil {
		t.Fatalf("configure: %v", err)
	}
	return p
}

func liveSandbox(ctx context.Context, t *testing.T, p *e2b.Provider, spec sandbox.Spec) sandbox.ID {
	t.Helper()
	spec.Name += "-" + randomSuffix(t)
	id, err := p.Create(ctx, spec)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		_ = p.Destroy(cleanup, id)
	})
	return id
}

// TestRealE2BCRUD creates, lists, inspects, execs in, restarts, and destroys
// one sandbox on the E2B API.
func TestRealE2BCRUD(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	id := liveSandbox(ctx, t, p, sandbox.Spec{Name: "kaxa-it", Labels: []string{"kaxa", "integration"}})

	infos, err := p.List(ctx)
	if err != nil || !infoHas(infos, id) {
		t.Fatalf("list = %+v err = %v", infos, err)
	}
	detail, err := p.Inspect(ctx, id)
	if err != nil || detail.Status != "running" || len(detail.Labels) != 2 {
		t.Fatalf("detail = %+v err = %v", detail, err)
	}

	var stdout, stderr strings.Builder
	err = p.Exec(ctx, id, sandbox.ExecOptions{
		Command: []string{"sh", "-c", "echo hi; echo oops >&2"}, Stdout: &stdout, Stderr: &stderr,
	})
	if err != nil || strings.TrimSpace(stdout.String()) != "hi" || strings.TrimSpace(stderr.String()) != "oops" {
		t.Fatalf("exec stdout = %q stderr = %q err = %v", stdout.String(), stderr.String(), err)
	}
	err = p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{"sh", "-c", "exit 3"}})
	if exitErr, ok := errors.AsType[*sandbox.ExitError](err); !ok || exitErr.Code != 3 {
		t.Fatalf("exec exit 3 err = %v", err)
	}

	var tty strings.Builder
	err = p.Exec(ctx, id, sandbox.ExecOptions{
		Command: []string{"sh", "-c", "stty size"}, Stdout: &tty, TTY: true, Size: sandbox.TermSize{Rows: 30, Cols: 100},
	})
	if err != nil || !strings.Contains(tty.String(), "30 100") {
		t.Fatalf("tty output = %q err = %v", tty.String(), err)
	}

	// A file survives a reboot from disk; a process does not.
	if err := p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{"sh", "-c", "echo kept > /home/user/kept.txt"}}); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := p.Restart(ctx, id); err != nil {
		t.Fatalf("restart: %v", err)
	}
	var kept strings.Builder
	if err := p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{"cat", "/home/user/kept.txt"}, Stdout: &kept}); err != nil || strings.TrimSpace(kept.String()) != "kept" {
		t.Fatalf("file after restart = %q err = %v", kept.String(), err)
	}

	if err := p.Destroy(ctx, id); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if _, err := p.Inspect(ctx, id); !errors.Is(err, e2b.ErrNotFound) {
		t.Fatalf("inspect after destroy err = %v", err)
	}
}

// TestRealE2BSync copies a tiny repository and installs its toolchain, which
// checks that the base template has what the installer needs (curl, tar, git).
func TestRealE2BSync(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	id := liveSandbox(ctx, t, p, sandbox.Spec{Name: "kaxa-it-sync", Labels: []string{"kaxa", sandbox.SyncLabel}})

	if err := p.SyncWorkspace(ctx, id, tinyRepoArchive(t)); err != nil {
		t.Fatalf("sync: %v", err)
	}
	var out strings.Builder
	err := p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{"sh", "-c", "pwd; id -un; cat demo/hello.txt"}, Stdout: &out})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if got := strings.Fields(out.String()); len(got) != 3 || got[0] != "/home/user/workspace" || got[1] != "user" || got[2] != "hi" {
		t.Fatalf("exec output = %q", out.String())
	}
	if ok, err := p.RepoExists(ctx, id, "demo"); err != nil || !ok {
		t.Fatalf("repo exists = %v err = %v", ok, err)
	}
	steps := []project.Step{{Kind: project.KindGo, Commands: [][]string{{"go", "version"}}}}
	if err := p.SetupProjects(ctx, id, "demo", steps); err != nil {
		t.Fatalf("setup: %v", err)
	}
}

// TestRealE2BPublishAPI starts a web server and checks that it keeps running
// after kaxa closes its stream to the sandbox.
func TestRealE2BPublishAPI(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	id := liveSandbox(ctx, t, p, sandbox.Spec{Name: "kaxa-it-api", Labels: []string{"kaxa"}, ExposeAPI: true, APIPort: 8000})
	argv := []string{"python3", "-m", "http.server", "8000", "--bind", "0.0.0.0"}
	if err := p.PublishAPI(ctx, id, "/home/user", 8000, argv, nil); err != nil {
		t.Fatalf("publish: %v", err)
	}
	var out strings.Builder
	err := p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{"python3", "-c", "import urllib.request as u; print(u.urlopen('http://127.0.0.1:8000/').status)"}, Stdout: &out})
	if err != nil || strings.TrimSpace(out.String()) != "200" {
		t.Fatalf("service after publish = %q err = %v", out.String(), err)
	}
	detail, err := p.Inspect(ctx, id)
	if err != nil || !strings.HasPrefix(detail.URL, "https://8000-") {
		t.Fatalf("detail = %+v err = %v", detail, err)
	}
}

// TestRealE2BTemplate creates a sandbox from the template named by
// KAXA_E2B_TEMPLATE, which must keep the base layout (user "user"), and reports
// its memory so a larger template can be checked by eye.
func TestRealE2BTemplate(t *testing.T) {
	template := os.Getenv("KAXA_E2B_TEMPLATE")
	if template == "" {
		t.Skip("KAXA_E2B_TEMPLATE is not set")
	}
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	id := liveSandbox(ctx, t, p, sandbox.Spec{Name: "kaxa-it-tpl", Labels: []string{"kaxa"}, Template: template})
	var out strings.Builder
	err := p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{"sh", "-c", "id -un; grep MemTotal /proc/meminfo"}, Stdout: &out})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if lines[0] != "user" {
		t.Fatalf("user = %q, want user: the template must keep the base layout", lines[0])
	}
	t.Logf("template %s: %s", template, lines[len(lines)-1])

	if _, err := p.Create(ctx, sandbox.Spec{Name: "kaxa-it-nope", Template: "kaxa-no-such-template"}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("create from a missing template err = %v", err)
	}
}

func TestRealE2BRejectsBadKey(t *testing.T) {
	if os.Getenv("KAXA_E2B_API_KEY") == "" {
		t.Skip("KAXA_E2B_API_KEY is not set; skipping network test")
	}
	p := e2b.New()
	if err := p.Configure("e2b_invalid_key"); err != nil {
		t.Fatalf("configure: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, err := p.List(ctx)
	if err == nil || errors.Is(err, e2b.ErrMissingCredentials) || strings.Contains(err.Error(), "e2b_invalid_key") {
		t.Fatalf("list with a bad key err = %v, want an API error without the key", err)
	}
}

func infoHas(infos []sandbox.Info, id sandbox.ID) bool {
	for _, i := range infos {
		if i.ID == id {
			return true
		}
	}
	return false
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return hex.EncodeToString(b)
}

// tinyRepoArchive is a gzip tar holding demo/hello.txt.
func tinyRepoArchive(t *testing.T) io.Reader {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("hi\n")
	if err := tw.WriteHeader(&tar.Header{Name: "demo/hello.txt", Mode: 0o644, Size: int64(len(body))}); err != nil {
		t.Fatalf("tar header: %v", err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("tar write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return &buf
}
