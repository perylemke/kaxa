//go:build live

package daytona_test

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
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/project"
	"github.com/perylemke/kaxa/internal/provider/daytona"
	"github.com/perylemke/kaxa/internal/sandbox"
)

// liveProvider returns a configured provider, or skips the test without
// KAXA_DAYTONA_API_KEY. These tests bill a few minutes of Daytona compute and
// the storage of one image. They are also the first check of the toolbox and
// image wire formats against the real service.
func liveProvider(t *testing.T) *daytona.Provider {
	t.Helper()
	key := os.Getenv("KAXA_DAYTONA_API_KEY")
	if key == "" {
		t.Skip("KAXA_DAYTONA_API_KEY is not set")
	}
	p := daytona.New()
	if err := p.Configure(key); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if err := p.SetDefaultTemplate(os.Getenv("KAXA_DAYTONA_TEMPLATE")); err != nil {
		t.Fatalf("template: %v", err)
	}
	return p
}

func liveSandbox(ctx context.Context, t *testing.T, p *daytona.Provider, spec sandbox.Spec) sandbox.ID {
	t.Helper()
	spec.Name += "-" + randomSuffix(t)
	id, err := p.Create(ctx, spec)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 2*time.Minute)
		defer stop()
		_ = p.Destroy(cleanup, id)
	})
	return id
}

// TestRealDaytonaCRUD creates, lists, inspects, execs in, restarts, and
// destroys one sandbox on the Daytona API.
func TestRealDaytonaCRUD(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
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

	// The sandbox is persistent: a file survives a stop and start.
	if err := p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{"sh", "-c", "echo kept > $HOME/kept.txt"}}); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := p.Restart(ctx, id); err != nil {
		t.Fatalf("restart: %v", err)
	}
	var kept strings.Builder
	if err := p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{"sh", "-c", "cat $HOME/kept.txt"}, Stdout: &kept}); err != nil || strings.TrimSpace(kept.String()) != "kept" {
		t.Fatalf("file after restart = %q err = %v", kept.String(), err)
	}

	if err := p.Destroy(ctx, id); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if _, err := p.Inspect(ctx, id); !errors.Is(err, daytona.ErrNotFound) {
		t.Fatalf("inspect after destroy err = %v", err)
	}
}

// TestRealDaytonaSync copies a tiny repository and installs its toolchain.
func TestRealDaytonaSync(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
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
	if got := strings.Fields(out.String()); len(got) != 3 || got[0] != "/home/daytona/workspace" || got[1] != "daytona" || got[2] != "hi" {
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

// TestRealDaytonaPublishAPI starts a web server and checks that it keeps
// running after kaxa closes its stream, and that the preview URL is public.
func TestRealDaytonaPublishAPI(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	id := liveSandbox(ctx, t, p, sandbox.Spec{Name: "kaxa-it-api", Labels: []string{"kaxa"}, ExposeAPI: true, APIPort: 8000})
	argv := []string{"python3", "-m", "http.server", "8000", "--bind", "0.0.0.0"}
	if err := p.PublishAPI(ctx, id, "/home/daytona", 8000, argv, nil); err != nil {
		t.Fatalf("publish: %v", err)
	}
	var out strings.Builder
	err := p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{"python3", "-c", "import urllib.request as u; print(u.urlopen('http://127.0.0.1:8000/').status)"}, Stdout: &out})
	if err != nil || strings.TrimSpace(out.String()) != "200" {
		t.Fatalf("service after publish = %q err = %v", out.String(), err)
	}
	detail, err := p.Inspect(ctx, id)
	if err != nil || !strings.HasPrefix(detail.URL, "https://") || detail.Auth != sandbox.URLAuthPublic {
		t.Fatalf("detail = %+v err = %v", detail, err)
	}
}

// TestRealDaytonaSnapshot captures a sandbox disk as an image, restores it into
// a new sandbox, and checks the file written before the snapshot.
func TestRealDaytonaSnapshot(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	id := liveSandbox(ctx, t, p, sandbox.Spec{Name: "kaxa-it-snap", Labels: []string{"kaxa", sandbox.SyncLabel}})
	if err := p.Exec(ctx, id, sandbox.ExecOptions{Command: []string{"sh", "-c", "echo snap > $HOME/snap.txt"}}); err != nil {
		t.Fatalf("write file: %v", err)
	}

	snap, err := p.Snapshot(ctx, id, "live test")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 2*time.Minute)
		defer stop()
		_ = p.DeleteSnapshot(cleanup, id, snap.ID)
	})
	snaps, err := p.ListSnapshots(ctx, id)
	if err != nil || len(snaps) != 1 || snaps[0].Comment != "live test" {
		t.Fatalf("snapshots = %+v err = %v", snaps, err)
	}

	target := sandbox.ID("kaxa-it-restore-" + randomSuffix(t))
	restored, err := p.Restore(ctx, sandbox.RestoreRequest{Sandbox: id, Snapshot: snap.ID, Target: target})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 2*time.Minute)
		defer stop()
		_ = p.Destroy(cleanup, restored)
	})
	var out strings.Builder
	if err := p.Exec(ctx, restored, sandbox.ExecOptions{Command: []string{"sh", "-c", "cat $HOME/snap.txt"}, Stdout: &out}); err != nil || strings.TrimSpace(out.String()) != "snap" {
		t.Fatalf("file in restored sandbox = %q err = %v", out.String(), err)
	}
}

func TestRealDaytonaRejectsBadKey(t *testing.T) {
	if os.Getenv("KAXA_DAYTONA_API_KEY") == "" {
		t.Skip("KAXA_DAYTONA_API_KEY is not set; skipping network test")
	}
	const bad = "dtn_invalid_key"
	p := daytona.New()
	if err := p.Configure(bad); err != nil {
		t.Fatalf("configure: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, err := p.List(ctx)
	if err == nil || errors.Is(err, daytona.ErrMissingCredentials) || strings.Contains(err.Error(), bad) {
		t.Fatalf("list with a bad key err = %v, want an API error without the key", err)
	}
}

func infoHas(infos []sandbox.Info, id sandbox.ID) bool {
	return slices.ContainsFunc(infos, func(i sandbox.Info) bool { return i.ID == id })
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
