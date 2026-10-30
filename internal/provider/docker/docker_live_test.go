//go:build live

package docker

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/sandbox"
)

// liveProvider returns a provider with the real Docker credentials, or skips
// the test when KAXA_DOCKER_ID and KAXA_DOCKER_PAT are not set. These tests
// create cloud sandboxes, which bill compute to the Docker subscription.
func liveProvider(t *testing.T) *Provider {
	t.Helper()
	id, pat := os.Getenv("KAXA_DOCKER_ID"), os.Getenv("KAXA_DOCKER_PAT")
	if id == "" || pat == "" {
		t.Skip("KAXA_DOCKER_ID and KAXA_DOCKER_PAT are not set")
	}
	p := New()
	if err := p.ConfigureCredentials(id, pat); err != nil {
		t.Fatal(err)
	}
	if tpl := os.Getenv("KAXA_DOCKER_TEMPLATE"); tpl != "" {
		if err := p.SetDefaultTemplate(tpl); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func liveName(t *testing.T) string {
	t.Helper()
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return "kaxa-it-" + hex.EncodeToString(b[:])
}

// TestRealDockerCRUD creates, lists, inspects, execs in, restarts, and
// destroys one sandbox with the real Docker Sandboxes API.
func TestRealDockerCRUD(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	id, err := p.Create(ctx, sandbox.Spec{Name: liveName(t), Labels: []string{labelKaxa, sandbox.SyncLabel}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 2*time.Minute)
		defer stop()
		_ = p.Destroy(cleanup, id)
	})

	detail, err := p.Inspect(ctx, id)
	if err != nil || detail.Status != statusRunning {
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

	if err := p.SyncWorkspace(ctx, id, helloArchive(t)); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if ok, err := p.RepoExists(ctx, id, "demo"); err != nil || !ok {
		t.Fatalf("repo exists = %v err = %v", ok, err)
	}
	stdout.Reset()
	err = p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{"cat", workspaceDir + "/demo/hello.txt"}, Stdout: &stdout})
	if err != nil || strings.TrimSpace(stdout.String()) != "hello" {
		t.Fatalf("synced file = %q err = %v", stdout.String(), err)
	}

	if err := p.Restart(ctx, id); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if err := p.Destroy(ctx, id); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if _, err := p.Inspect(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("inspect after destroy err = %v", err)
	}
}

// helloArchive is a gzip tar with demo/hello.txt, the shape reposync produces.
func helloArchive(t *testing.T) *bytes.Reader {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("hello\n")
	for _, h := range []*tar.Header{
		{Name: "demo/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "demo/hello.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(body))},
	} {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write(body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(buf.Bytes())
}

// TestRealDockerPublishAPI starts a web server in a sandbox, publishes its
// port and requests the URL that Docker returns. It checks three things that
// only Docker can answer: that the service survives the end of the process
// stream, that the token may publish ports, and whether the URL is open.
func TestRealDockerPublishAPI(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	id, err := p.Create(ctx, sandbox.Spec{Name: liveName(t), Labels: []string{labelKaxa}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 2*time.Minute)
		defer stop()
		_ = p.Destroy(cleanup, id)
	})

	argv := []string{"python3", "-m", "http.server", "8000", "--bind", "0.0.0.0"}
	if err := p.PublishAPI(ctx, id, agentHome, 8000, argv, nil); err != nil {
		t.Fatalf("publish: %v", err)
	}
	detail, err := p.Inspect(ctx, id)
	if err != nil || detail.URL == "" {
		t.Fatalf("detail = %+v err = %v", detail, err)
	}
	t.Logf("published URL: %s", detail.URL)

	// The service must still run now that the stream to it is closed.
	var stdout strings.Builder
	err = p.Exec(ctx, id, sandbox.ExecOptions{
		Command: []string{"bash", "-c", `exec 3<>/dev/tcp/127.0.0.1/8000 && echo up`}, Stdout: &stdout,
	})
	if err != nil || strings.TrimSpace(stdout.String()) != "up" {
		t.Fatalf("service is not listening: out = %q err = %v", stdout.String(), err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, detail.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request %s: %v", detail.URL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	t.Logf("GET %s -> %s", detail.URL, resp.Status)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %s", resp.Status)
	}
}
