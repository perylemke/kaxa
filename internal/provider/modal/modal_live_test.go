//go:build live

package modal_test

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
	"github.com/perylemke/kaxa/internal/provider/modal"
	"github.com/perylemke/kaxa/internal/sandbox"
)

// TestRealModalCRUD creates, lists, inspects, and destroys one sandbox on the
// Modal API. It is skipped unless KAXA_MODAL_TOKEN_ID and
// KAXA_MODAL_TOKEN_SECRET are set, and it bills a few seconds of compute.
func TestRealModalCRUD(t *testing.T) {
	id, secret := os.Getenv("KAXA_MODAL_TOKEN_ID"), os.Getenv("KAXA_MODAL_TOKEN_SECRET")
	if id == "" || secret == "" {
		t.Skip("KAXA_MODAL_TOKEN_ID and KAXA_MODAL_TOKEN_SECRET are not set")
	}

	p := modal.New()
	if err := p.ConfigureCredentials(id, secret); err != nil {
		t.Fatalf("configure: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	name := "kaxa-it-" + randomSuffix(t)
	sid, err := p.Create(ctx, sandbox.Spec{Name: name, Labels: []string{"kaxa", "integration"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		_ = p.Destroy(cleanup, sid)
	})

	if _, err := p.Create(ctx, sandbox.Spec{Name: name}); err == nil {
		t.Fatal("duplicate create succeeded")
	}

	infos, err := p.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !containsID(infos, sid) {
		t.Fatalf("list %+v does not contain %q", infos, sid)
	}

	detail, err := p.Inspect(ctx, sid)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if detail.ID != sid || detail.Status != "running" || len(detail.Labels) != 2 {
		t.Fatalf("detail = %+v", detail)
	}

	var stdout, stderr strings.Builder
	err = p.Exec(ctx, sid, sandbox.ExecOptions{
		Command: []string{"sh", "-c", "echo hi; echo oops >&2"},
		Stdout:  &stdout,
		Stderr:  &stderr,
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if strings.TrimSpace(stdout.String()) != "hi" || strings.TrimSpace(stderr.String()) != "oops" {
		t.Fatalf("exec stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}

	err = p.Exec(ctx, sid, sandbox.ExecOptions{Command: []string{"sh", "-c", "exit 3"}})
	if exitErr, ok := errors.AsType[*sandbox.ExitError](err); !ok || exitErr.Code != 3 {
		t.Fatalf("exec exit 3 err = %v, want exit code 3", err)
	}

	var tty strings.Builder
	err = p.Exec(ctx, sid, sandbox.ExecOptions{
		Command: []string{"sh", "-c", "stty size"},
		Stdout:  &tty,
		TTY:     true,
		Size:    sandbox.TermSize{Rows: 30, Cols: 100},
	})
	if err != nil {
		t.Fatalf("exec tty: %v", err)
	}
	if !strings.Contains(tty.String(), "30 100") {
		t.Fatalf("tty size output = %q, want 30 100", tty.String())
	}

	if err := p.Destroy(ctx, sid); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if _, err := p.Inspect(ctx, sid); err == nil {
		t.Fatal("inspect after destroy succeeded")
	}
	infos, err = p.List(ctx)
	if err != nil {
		t.Fatalf("list after destroy: %v", err)
	}
	if containsID(infos, sid) {
		t.Fatalf("list still contains %q", sid)
	}
}

func TestRealModalRejectsBadToken(t *testing.T) {
	if os.Getenv("KAXA_MODAL_TOKEN_ID") == "" {
		t.Skip("KAXA_MODAL_TOKEN_ID is not set; skipping network test")
	}
	p := modal.New()
	if err := p.ConfigureCredentials("ak-invalid", "as-invalid"); err != nil {
		t.Fatalf("configure: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, err := p.List(ctx); err == nil || errors.Is(err, modal.ErrMissingCredentials) {
		t.Fatalf("list with a bad token err = %v, want an API error", err)
	}
}

func containsID(infos []sandbox.Info, id sandbox.ID) bool {
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

// TestRealModalSync creates a sandbox with the sync label, which builds the
// sync image on first use, copies a tiny repository, and installs its toolchain.
// It is skipped without credentials and can take several minutes the first time.
func TestRealModalSync(t *testing.T) {
	id, secret := os.Getenv("KAXA_MODAL_TOKEN_ID"), os.Getenv("KAXA_MODAL_TOKEN_SECRET")
	if id == "" || secret == "" {
		t.Skip("KAXA_MODAL_TOKEN_ID and KAXA_MODAL_TOKEN_SECRET are not set")
	}

	p := modal.New()
	if err := p.ConfigureCredentials(id, secret); err != nil {
		t.Fatalf("configure: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	name := "kaxa-it-sync-" + randomSuffix(t)
	sid, err := p.Create(ctx, sandbox.Spec{Name: name, Labels: []string{"kaxa", sandbox.SyncLabel}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		_ = p.Destroy(cleanup, sid)
	})

	if err := p.SyncWorkspace(ctx, sid, tinyRepoArchive(t)); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var out strings.Builder
	err = p.Exec(ctx, sid, sandbox.ExecOptions{
		Command: []string{"sh", "-c", "pwd; id -un; cat demo/hello.txt"},
		Stdout:  &out,
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if got := strings.Fields(out.String()); len(got) != 3 || got[0] != "/home/modal/workspace" || got[1] != "modal" || got[2] != "hi" {
		t.Fatalf("exec output = %q, want workspace, modal, hi", out.String())
	}

	steps := []project.Step{{Kind: project.KindGo, Commands: [][]string{{"go", "version"}}}}
	if err := p.SetupProjects(ctx, sid, "demo", steps); err != nil {
		t.Fatalf("setup: %v", err)
	}
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
