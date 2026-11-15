package cli_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/cli"
	"github.com/perylemke/kaxa/internal/registry"
)

type safeBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *safeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *safeBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func TestUIRejectsAnInvalidPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "not a number", args: []string{"ui", "--port=nope"}},
		{name: "too high", args: []string{"ui", "--port", "70000"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cmd := cli.NewRootCmd(registry.New())
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestUIPrintsTheAddressAndSkipsTheBrowser(t *testing.T) {
	opened := false
	restore := cli.SetBrowserExecCommandForTest(func(string, ...string) *exec.Cmd {
		opened = true
		return exec.Command("true")
	})
	t.Cleanup(restore)

	stderr := &safeBuf{}
	ctx, cancel := context.WithCancel(t.Context())
	cmd := cli.NewRootCmd(registry.New())
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"ui", "--no-open"})
	done := make(chan error, 1)
	go func() { done <- cmd.Execute() }()

	page := waitListen(t, stderr)
	res := getReady(t, page)
	body, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || !bytes.Contains(body, []byte("kaxa-session")) {
		t.Fatalf("page = %d", res.StatusCode)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if opened {
		t.Fatal("browser was opened")
	}
}

func TestUIOpensTheLoopbackAddress(t *testing.T) {
	var mu sync.Mutex
	var opened string
	restore := cli.SetBrowserExecCommandForTest(func(_ string, args ...string) *exec.Cmd {
		mu.Lock()
		opened = strings.Join(args, " ")
		mu.Unlock()
		return exec.Command("true")
	})
	t.Cleanup(restore)

	stderr := &safeBuf{}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cmd := cli.NewRootCmd(registry.New())
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"ui"})
	done := make(chan error, 1)
	go func() { done <- cmd.Execute() }()

	page := waitListen(t, stderr)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := opened
		mu.Unlock()
		if strings.Contains(got, page) {
			cancel()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("opened %q, stderr %s", opened, stderr.String())
}

func waitListen(t *testing.T, buf *safeBuf) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		text := buf.String()
		const prefix = "http://127.0.0.1:"
		_, after, ok := strings.Cut(text, prefix)
		if ok {
			line, _, found := strings.Cut(after, "\n")
			if found {
				return strings.TrimSpace(prefix + line)
			}
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("address not printed: %s", buf.String())
	return ""
}

func getReady(t *testing.T, page string) *http.Response {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		res, err := http.Get(page) // #nosec G107 -- page is the loopback URL printed by the server under test
		if err == nil {
			return res
		}
		last = err
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatal(last)
	return nil
}
