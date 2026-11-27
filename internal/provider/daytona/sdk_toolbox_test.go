package daytona

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestSDKToolboxUpload(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	tb := f.toolbox()
	if err := tb.Upload(context.Background(), sandboxID, "/tmp/x", strings.NewReader("data")); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if !f.seen("/files/bulk-upload") {
		t.Error("no upload request")
	}
}

func TestSDKToolboxUploadErrors(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	f.override = func(r *http.Request) (int, any, bool) {
		if strings.Contains(r.URL.Path, "/files") {
			return http.StatusInternalServerError, errBody(500, "disk"), true
		}
		return 0, nil, false
	}
	var api *APIError
	if err := f.toolbox().Upload(context.Background(), sandboxID, "/x", strings.NewReader("d")); !errors.As(err, &api) {
		t.Fatalf("Upload = %v, want API error", err)
	}

	g := newFakeServer(t)
	g.override = func(*http.Request) (int, any, bool) {
		return http.StatusNotFound, errBody(404, "gone"), true
	}
	tb := g.toolbox()
	ctx := context.Background()
	if err := tb.Upload(ctx, sandboxID, "/x", strings.NewReader("d")); !errors.Is(err, ErrNotFound) {
		t.Errorf("Upload on missing sandbox = %v", err)
	}
	if _, err := tb.Run(ctx, sandboxID, runSpec{argv: []string{"ls"}}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Run on missing sandbox = %v", err)
	}
	if _, err := tb.Spawn(ctx, sandboxID, runSpec{argv: []string{"ls"}}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Spawn on missing sandbox = %v", err)
	}
}

func TestSDKToolboxRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		exit     any
		stdin    string
		wantCode int
	}{
		{"success", float64(0), "", 0},
		{"failure code", float64(7), "", 7},
		{"with input", float64(0), "payload", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFakeServer(t)
			f.stdout, f.stderr, f.exit = "out-text", "err-text", tt.exit
			var spec runSpec
			var stdout, stderr bytes.Buffer
			spec = runSpec{argv: []string{echoCmd, "hi"}, dir: "/w", env: []string{"A=1"}, stdout: &stdout, stderr: &stderr}
			if tt.stdin != "" {
				spec.stdin = strings.NewReader(tt.stdin)
			}
			code, err := f.toolbox().Run(context.Background(), sandboxID, spec)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			expect(t, "code", code, tt.wantCode)
			expect(t, "stdout", stdout.String(), "out-text")
			expect(t, "stderr", stderr.String(), "err-text")
			if !f.seen("DELETE /tb/sb-1/process/session/") {
				t.Error("session was not deleted")
			}
			if tt.stdin != "" && !f.seen("/files/bulk-upload") {
				t.Error("input was not staged")
			}
			var exec struct{ Command string }
			_ = json.Unmarshal([]byte(f.body("POST /tb/sb-1/process/session/"+sessionOf(f)+"/exec")), &exec)
			if !strings.Contains(exec.Command, "cd '/w' && env 'A=1' 'echo' 'hi'") {
				t.Errorf("command = %s", exec)
			}
		})
	}
}

// sessionOf finds the session ID of the first recorded session exec.
func sessionOf(f *fakeServer) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.requests {
		if strings.HasSuffix(r, "/exec") && strings.Contains(r, "/process/session/") {
			_, rest, _ := strings.Cut(r, "/process/session/")
			return strings.TrimSuffix(rest, "/exec")
		}
	}
	return ""
}

func TestSDKToolboxRunEmptyInputIsNotStaged(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	_, err := f.toolbox().Run(context.Background(), sandboxID, runSpec{argv: []string{"cat"}, stdin: strings.NewReader("")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if f.seen("/files/bulk-upload") {
		t.Error("empty input was uploaded")
	}
}

func TestSDKToolboxRunWaitsForExit(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	var polls atomic.Int32
	f.override = func(r *http.Request) (int, any, bool) {
		if strings.Contains(r.URL.Path, "/command/") && !strings.HasSuffix(r.URL.Path, "/logs") {
			if polls.Add(1) < 3 {
				return http.StatusOK, map[string]any{"id": cmdID1, "command": "x"}, true
			}
			return http.StatusOK, map[string]any{"id": cmdID1, "command": "x", "exitCode": 0}, true
		}
		return 0, nil, false
	}
	f.stdout = "done"
	var out bytes.Buffer
	code, err := f.toolbox().Run(context.Background(), sandboxID, runSpec{argv: []string{"x"}, stdout: &out})
	if err != nil || code != 0 {
		t.Fatalf("Run = %d, %v", code, err)
	}
	expect(t, "stdout", out.String(), "done")
	if polls.Load() < 3 {
		t.Errorf("polls = %d", polls.Load())
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("sink closed") }

func TestSDKToolboxRunReportsWriteError(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	f.stdout = "x"
	_, err := f.toolbox().Run(context.Background(), sandboxID, runSpec{argv: []string{"x"}, stdout: failWriter{}})
	if err == nil || !strings.Contains(err.Error(), "sink closed") {
		t.Fatalf("err = %v", err)
	}
}

func TestSDKToolboxRunFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// fail is a path fragment; the request fails when it matches.
		fail  string
		stdin string
		exit  any
		want  string
	}{
		{"stage upload", "/files/bulk-upload", "in", nil, "500"},
		{"create session", "/process/session", "", nil, "500"},
		{"execute", "/exec", "", nil, "500"},
		{keyStatus, "/command/", "", nil, "500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFakeServer(t)
			if tt.exit != nil {
				f.exit = tt.exit
			}
			f.override = func(r *http.Request) (int, any, bool) {
				sessionOp := tt.fail == "/process/session" && (r.Method == http.MethodDelete || strings.Contains(r.URL.Path, "/exec"))
				failing := tt.fail != "" && strings.Contains(r.URL.Path, tt.fail) && !sessionOp
				if failing {
					return http.StatusInternalServerError, errBody(500, msgBoom), true
				}
				return 0, nil, false
			}
			spec := runSpec{argv: []string{"x"}}
			if tt.stdin != "" {
				spec.stdin = strings.NewReader(tt.stdin)
			}
			_, err := f.toolbox().Run(context.Background(), sandboxID, spec)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestSDKToolboxRunLogsFailure(t *testing.T) {
	t.Parallel()
	for _, done := range []bool{true, false} {
		name := "while running"
		if done {
			name = "after exit"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFakeServer(t)
			if !done {
				f.exit = nil
			}
			f.override = func(r *http.Request) (int, any, bool) {
				if strings.HasSuffix(r.URL.Path, "/logs") {
					return http.StatusInternalServerError, errBody(500, msgBoom), true
				}
				return 0, nil, false
			}
			if _, err := f.toolbox().Run(context.Background(), sandboxID, runSpec{argv: []string{"x"}}); err == nil {
				t.Fatal("Run succeeded")
			}
		})
	}
}

func TestSDKToolboxRunStopsWhenContextEnds(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	f.exit = nil
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(60*time.Millisecond, cancel)
	if _, err := f.toolbox().Run(ctx, sandboxID, runSpec{argv: []string{"x"}}); err == nil {
		t.Fatal("Run succeeded after cancel")
	}
}

func TestSDKToolboxRunRejectsHugeInput(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	huge := io.MultiReader(strings.NewReader("x"), zeroReader{n: maxStagedInput + 1})
	_, err := f.toolbox().Run(context.Background(), sandboxID, runSpec{argv: []string{"x"}, stdin: huge})
	if err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("err = %v", err)
	}
	if !f.seen("POST /tb/sb-1/process/execute") {
		t.Error("staged input was not removed")
	}
}

type zeroReader struct{ n int }

func (z zeroReader) Read(p []byte) (int, error) {
	return len(p), nil
}

func TestSDKToolboxSpawn(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	f.stdout = strings.Repeat("o", failureOutput+10)
	f.exit = float64(3)
	proc, err := f.toolbox().Spawn(context.Background(), sandboxID, runSpec{argv: []string{"serve"}})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	code, done, err := proc.Wait(context.Background(), time.Second)
	if err != nil || !done {
		t.Fatalf("Wait = %d, %v, %v", code, done, err)
	}
	expect(t, "code", code, 3)
	expect(t, "output length", len(proc.Output(context.Background())), failureOutput)
}

func TestSDKToolboxSpawnWaitTimesOut(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	f.exit = nil
	proc, err := f.toolbox().Spawn(context.Background(), sandboxID, runSpec{argv: []string{"serve"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, done, err := proc.Wait(context.Background(), 10*time.Millisecond); done || err != nil {
		t.Fatalf("Wait = %v, %v", done, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := proc.Wait(ctx, time.Minute); err == nil {
		t.Fatal("Wait ignored a cancelled context")
	}
}

func TestSDKToolboxSpawnFailures(t *testing.T) {
	t.Parallel()
	for _, fail := range []string{"/exec", "/process/session"} {
		t.Run(fail, func(t *testing.T) {
			t.Parallel()
			f := newFakeServer(t)
			f.override = func(r *http.Request) (int, any, bool) {
				ok := strings.HasSuffix(r.URL.Path, fail) && (fail == "/exec" || r.Method == http.MethodPost)
				return http.StatusInternalServerError, errBody(500, msgBoom), ok
			}
			if _, err := f.toolbox().Spawn(context.Background(), sandboxID, runSpec{argv: []string{"x"}}); err == nil {
				t.Fatal("Spawn succeeded")
			}
		})
	}
}

func TestSessionProcessOutputFailureIsEmpty(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	f.override = func(r *http.Request) (int, any, bool) {
		return http.StatusInternalServerError, errBody(500, msgBoom), strings.HasSuffix(r.URL.Path, "/logs")
	}
	proc, err := f.toolbox().Spawn(context.Background(), sandboxID, runSpec{argv: []string{"x"}})
	if err != nil {
		t.Fatal(err)
	}
	expect(t, "output", proc.Output(context.Background()), "")
}

func TestCommandStatusExitCodeTypes(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	f.exit = float64(9)
	sb, err := f.client().get(context.Background(), sandboxID)
	if err != nil {
		t.Fatal(err)
	}
	code, done, err := commandStatus(context.Background(), sb, "s", "c")
	if err != nil || !done || code != 9 {
		t.Fatalf("commandStatus = %d, %v, %v", code, done, err)
	}
}

// ptyScript runs a terminal session against the fake: it sends output, then ends
// the command with exit (a JSON-ready value, nil for no code).
func ptyScript(t *testing.T, got chan<- string, exit any, wait <-chan struct{}) func(*websocket.Conn) {
	t.Helper()
	return func(c *websocket.Conn) {
		_ = c.WriteJSON(map[string]any{"type": "control", keyStatus: "connected"})
		_ = c.WriteMessage(websocket.BinaryMessage, []byte("hello"))
		_, in, err := c.ReadMessage()
		if err == nil && got != nil {
			got <- string(in)
		}
		if wait != nil {
			<-wait
		}
		msg := map[string]any{"type": "control", keyStatus: "exited"}
		if exit != nil {
			msg["exitCode"] = exit
		}
		_ = c.WriteJSON(msg)
	}
}

func TestSDKToolboxRunTerminal(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	got := make(chan string, 1)
	f.ptyHandler = ptyScript(t, got, 4, nil)
	var out bytes.Buffer
	code, err := f.toolbox().Run(context.Background(), sandboxID, runSpec{
		argv: []string{bashCmd}, tty: true, dir: "/w", stdout: &out, stdin: strings.NewReader("typed"),
		size: sandbox.TermSize{Rows: 30, Cols: 100},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	expect(t, "code", code, 4)
	expect(t, "output", out.String(), "hello")
	if line := <-got; !strings.Contains(line, "cd '/w' && exec 'bash'") {
		t.Errorf("first input = %q", line)
	}
}

func TestSDKToolboxRunTerminalResizes(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	resized := make(chan struct{}, 1)
	f.override = func(r *http.Request) (int, any, bool) {
		if strings.HasSuffix(r.URL.Path, "/resize") {
			select {
			case resized <- struct{}{}:
			default:
			}
		}
		return 0, nil, false
	}
	f.ptyHandler = ptyScript(t, nil, 0, resized)
	resize := make(chan sandbox.TermSize, 2)
	resize <- sandbox.TermSize{}
	resize <- sandbox.TermSize{Rows: 50, Cols: 120}
	code, err := f.toolbox().Run(context.Background(), sandboxID, runSpec{
		argv: []string{bashCmd}, tty: true, stdout: failWriter{}, resize: resize,
	})
	if code != 0 || err == nil || !strings.Contains(err.Error(), "sink closed") {
		t.Fatalf("Run = %d, %v", code, err)
	}
}

func TestSDKToolboxRunTerminalWithoutExitCode(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	f.ptyHandler = func(c *websocket.Conn) {
		_ = c.WriteJSON(map[string]any{"type": "control", keyStatus: "exited"})
		_ = c.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseAbnormalClosure, ""), time.Now().Add(time.Second))
	}
	_, err := f.toolbox().Run(context.Background(), sandboxID, runSpec{argv: []string{bashCmd}, tty: true})
	if err == nil {
		t.Fatal("Run succeeded without an exit code")
	}
}

func TestSDKToolboxRunTerminalConnectFails(t *testing.T) {
	t.Parallel()
	f := newFakeServer(t)
	f.override = func(r *http.Request) (int, any, bool) {
		return http.StatusInternalServerError, map[string]any{}, strings.HasSuffix(r.URL.Path, "/create-connect")
	}
	f.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/create-connect") {
			http.Error(w, "no", http.StatusInternalServerError)
			return
		}
		f.serve(w, r)
	})
	if _, err := f.toolbox().Run(context.Background(), sandboxID, runSpec{argv: []string{bashCmd}, tty: true}); err == nil {
		t.Fatal("Run succeeded")
	}
}

func TestWatchResizeAndPumpStopCleanly(t *testing.T) {
	t.Parallel()
	watchResize(context.Background(), nil, nil)
	pumpTerminal(nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	watchResize(ctx, nil, make(chan sandbox.TermSize))

	closed := make(chan sandbox.TermSize)
	close(closed)
	watchResize(context.Background(), nil, closed)
}
