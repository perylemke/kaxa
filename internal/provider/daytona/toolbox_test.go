package daytona

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func toolboxFixture(t *testing.T, h http.HandlerFunc) (*httpToolbox, Conn) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return newHTTPToolbox(testKey), Conn{SandboxID: sandboxID, BaseURL: srv.URL}
}

func TestProcessBaseRejectsUnsafeTargets(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		conn Conn
		ok   bool
	}{
		{"https", Conn{SandboxID: "sb", BaseURL: "https://toolbox.daytona.com"}, true},
		{"default url", Conn{SandboxID: "sb"}, true},
		{"plain http", Conn{SandboxID: "sb", BaseURL: "http://toolbox.daytona.com"}, false},
		{"path in id", Conn{SandboxID: "../x", BaseURL: "https://toolbox.daytona.com"}, false},
		{"empty id", Conn{BaseURL: "https://toolbox.daytona.com"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := processBase(tt.conn)
			if (err == nil) != tt.ok {
				t.Fatalf("processBase err = %v, want ok=%v", err, tt.ok)
			}
		})
	}
}

func TestToolboxStartSendsArgvAndAuth(t *testing.T) {
	t.Parallel()
	var got map[string]any
	tb, conn := toolboxFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testKey {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Method != http.MethodPost || r.URL.Path != "/toolbox/sb-1/processes" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"id":"p1"}`))
	})
	id, err := tb.Start(t.Context(), conn, ProcessRequest{Kind: kindPTY, Argv: []string{bashCmd, "-l"}, Dir: "/w", Stdin: true, Rows: 24, Cols: 80})
	if err != nil || id != "p1" {
		t.Fatalf("Start = %q, %v", id, err)
	}
	argv, _ := got["argv"].([]any)
	term, _ := got["terminal"].(map[string]any)
	if got["kind"] != "pty" || len(argv) != 2 || got["cwd"] != "/w" || got["stdin"] != "open" || term["cols"] != float64(80) {
		t.Fatalf("body = %v", got)
	}
	if _, hasShell := got["shellCommand"]; hasShell {
		t.Fatal("a shell command was sent")
	}
}

func TestToolboxExecHasNoTerminalAndClosedStdin(t *testing.T) {
	t.Parallel()
	var got map[string]any
	tb, conn := toolboxFixture(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"id":"p1"}`))
	})
	if _, err := tb.Start(t.Context(), conn, ProcessRequest{Kind: kindExec, Argv: []string{"ls"}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["terminal"]; ok || got["stdin"] != "closed" {
		t.Fatalf("body = %v", got)
	}
}

func TestToolboxLogsAndWait(t *testing.T) {
	t.Parallel()
	tb, conn := toolboxFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/logs"):
			if r.URL.Query().Get("cursor") != "c1" {
				t.Errorf("cursor = %q", r.URL.Query().Get("cursor"))
			}
			_, _ = w.Write([]byte(`{"items":[{"channel":"stdout","data":"a","cursor":"c2"},{"stream":"stderr","data":"b","cursor":"c3"}]}`))
		case strings.HasSuffix(r.URL.Path, "/wait"):
			if r.URL.Query().Get("timeoutMs") != "750" {
				t.Errorf("timeoutMs = %q", r.URL.Query().Get("timeoutMs"))
			}
			_, _ = w.Write([]byte(`{"reason":"exited","exitCode":4}`))
		}
	})
	frames, err := tb.Logs(t.Context(), conn, "p1", "c1")
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	expect(t, "Logs", frames, []LogFrame{{Channel: "stdout", Data: "a", Cursor: "c2"}, {Channel: "stderr", Data: "b", Cursor: "c3"}})
	res, err := tb.Wait(t.Context(), conn, "p1", 750*time.Millisecond)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	expect(t, "Wait", []any{res.Done(), res.HasCode, res.ExitCode}, []any{true, true, 4})
}

func TestWaitTimedOutIsNotDone(t *testing.T) {
	t.Parallel()
	tb, conn := toolboxFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"reason":"timed_out"}`))
	})
	res, err := tb.Wait(t.Context(), conn, "p1", time.Second)
	if err != nil || res.Done() || res.HasCode {
		t.Fatalf("Wait = %+v, %v", res, err)
	}
}

func TestToolboxStdinResizeAndUpload(t *testing.T) {
	t.Parallel()
	var stdin map[string]any
	var resize map[string]float64
	var uploaded, uploadPath string
	tb, conn := toolboxFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/stdin"):
			_ = json.NewDecoder(r.Body).Decode(&stdin)
		case strings.HasSuffix(r.URL.Path, "/resize"):
			_ = json.NewDecoder(r.Body).Decode(&resize)
		case strings.HasSuffix(r.URL.Path, "/files/upload"):
			uploadPath = r.URL.Query().Get("path")
			f, _, err := r.FormFile("file")
			if err != nil {
				t.Errorf("FormFile: %v", err)
				return
			}
			data, _ := io.ReadAll(f)
			uploaded = string(data)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if err := tb.Stdin(t.Context(), conn, "p1", []byte("y\n"), true); err != nil {
		t.Fatal(err)
	}
	if err := tb.Resize(t.Context(), conn, "p1", 40, 150); err != nil {
		t.Fatal(err)
	}
	if err := tb.Upload(t.Context(), conn, "/tmp/a.tgz", strings.NewReader("archive")); err != nil {
		t.Fatal(err)
	}
	expect(t, "stdin", stdin, map[string]any{"data": "y\n", "eof": true})
	expect(t, "resize", resize, map[string]float64{"rows": 40, "cols": 150})
	expect(t, "upload", []string{uploaded, uploadPath}, []string{"archive", "/tmp/a.tgz"})
}

func TestToolboxErrorIsAPIError(t *testing.T) {
	t.Parallel()
	tb, conn := toolboxFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"sandbox not running"}`))
	})
	_, err := tb.Start(t.Context(), conn, ProcessRequest{Kind: kindExec, Argv: []string{"ls"}})
	if err == nil || !strings.Contains(err.Error(), "409") || strings.Contains(err.Error(), testKey) {
		t.Fatalf("err = %v", err)
	}
}
