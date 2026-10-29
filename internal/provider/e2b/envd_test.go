package e2b

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testDaemonToken = "daemon-token-123"

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func frame(flags byte, payload string) []byte {
	out := make([]byte, 5, 5+len(payload))
	out[0] = flags
	binary.BigEndian.PutUint32(out[1:], uint32(len(payload))) //gosec:disable G115 -- test payloads are tiny
	return append(out, payload...)
}

func newTestEnvd(t *testing.T, h http.HandlerFunc) (*httpEnvd, Connection) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	e := &httpEnvd{http: srv.Client(), baseURL: func(Connection) string { return srv.URL }}
	return e, Connection{ID: testSandbox, Domain: defaultDomain, AccessToken: testDaemonToken}
}

func writeFrames(w http.ResponseWriter, frames ...[]byte) {
	w.Header().Set("Content-Type", "application/connect+json")
	for _, f := range frames {
		_, _ = w.Write(f)
	}
}

func readStartBody(t *testing.T, r *http.Request) startBody {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil || len(raw) < 5 {
		t.Fatalf("read body: %v (%d bytes)", err, len(raw))
	}
	if int(binary.BigEndian.Uint32(raw[1:5])) != len(raw)-5 {
		t.Fatalf("envelope length does not match the body")
	}
	var body startBody
	if err := json.Unmarshal(raw[5:], &body); err != nil {
		t.Fatalf("decode start body: %v", err)
	}
	return body
}

func TestStartStreamsEventsAndExit(t *testing.T) {
	t.Parallel()

	var got startBody
	var header http.Header
	e, conn := newTestEnvd(t, func(w http.ResponseWriter, r *http.Request) {
		got, header = readStartBody(t, r), r.Header.Clone()
		if r.URL.Path != "/process.Process/Start" {
			t.Errorf("path = %q", r.URL.Path)
		}
		writeFrames(w,
			frame(0, `{"event":{"start":{"pid":42}}}`),
			frame(0, `{"event":{"keepalive":{}}}`),
			frame(0, `{"event":{"data":{"stdout":"`+b64("out\n")+`"}}}`),
			frame(0, `{"event":{"data":{"stderr":"`+b64("err\n")+`"}}}`),
			frame(0, `{"event":{"end":{"exitCode":3,"exited":true,"status":"exit status 3"}}}`),
			frame(endStreamFlag, `{}`),
		)
	})

	p, err := e.Start(context.Background(), conn, StartRequest{
		Cmd: "env", Args: []string{"A=1", "ls"}, Env: map[string]string{"K": "V"}, Cwd: "/work", User: "user", Stdin: true,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer p.Close()

	kinds, out, last := drainEvents(t, p)
	if len(kinds) != 3 || kinds[0] != EventStdout || kinds[1] != EventStderr || kinds[2] != EventEnd {
		t.Fatalf("kinds = %v", kinds)
	}
	if out != "out\nerr\n" || last.ExitCode != 3 {
		t.Fatalf("output = %q last = %+v", out, last)
	}
	assertStartRequest(t, got, header)
	if _, err := p.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("next after end = %v", err)
	}
}

func drainEvents(t *testing.T, p Process) ([]EventKind, string, Event) {
	t.Helper()
	var kinds []EventKind
	var out strings.Builder
	var last Event
	for {
		ev, err := p.Next()
		if errors.Is(err, io.EOF) {
			return kinds, out.String(), last
		}
		if err != nil {
			t.Fatalf("next: %v", err)
		}
		kinds = append(kinds, ev.Kind)
		out.Write(ev.Data)
		last = ev
	}
}

func assertStartRequest(t *testing.T, got startBody, header http.Header) {
	t.Helper()
	if got.Process.Cmd != "env" || len(got.Process.Args) != 2 || got.Process.Envs["K"] != "V" || got.Process.Cwd != "/work" || !got.Stdin || got.PTY != nil {
		t.Fatalf("start body = %+v", got)
	}
	if header.Get("X-Access-Token") != testDaemonToken || header.Get("Connect-Protocol-Version") != "1" || header.Get("Keepalive-Ping-Interval") != keepalivePing {
		t.Fatalf("headers = %v", header)
	}
	user, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(header.Get("Authorization"), "Basic "))
	if string(user) != "user:" {
		t.Fatalf("auth user = %q", user)
	}
}

func TestStartPTY(t *testing.T) {
	t.Parallel()

	var got startBody
	e, conn := newTestEnvd(t, func(w http.ResponseWriter, r *http.Request) {
		got = readStartBody(t, r)
		writeFrames(w,
			frame(0, `{"event":{"start":{"pid":7}}}`),
			frame(0, `{"event":{"data":{"pty":"`+b64("$ ")+`"}}}`),
			frame(0, `{"event":{"end":{"exited":true}}}`),
		)
	})

	p, err := e.Start(context.Background(), conn, StartRequest{Cmd: "bash", User: "user", PTY: true, Rows: 30, Cols: 100})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer p.Close()
	ev, err := p.Next()
	if err != nil || ev.Kind != EventPTY || string(ev.Data) != "$ " {
		t.Fatalf("event = %+v err = %v", ev, err)
	}
	end, err := p.Next()
	if err != nil || end.Kind != EventEnd || end.ExitCode != 0 {
		t.Fatalf("end = %+v err = %v", end, err)
	}
	if got.PTY == nil || got.PTY.Size.Rows != 30 || got.PTY.Size.Cols != 100 {
		t.Fatalf("pty = %+v", got.PTY)
	}
}

func TestStartErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		handler http.HandlerFunc
		wantMsg string
	}{
		{
			name: "http status",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"code":"unauthenticated","message":"bad token"}`))
			},
			wantMsg: "bad token",
		},
		{
			name: "error in the stream",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				writeFrames(w, frame(endStreamFlag, `{"error":{"code":"not_found","message":"no such binary"}}`))
			},
			wantMsg: "no such binary",
		},
		{
			name:    "stream closes before start",
			handler: func(w http.ResponseWriter, _ *http.Request) { writeFrames(w) },
			wantMsg: "before the command started",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e, conn := newTestEnvd(t, tt.handler)
			_, err := e.Start(context.Background(), conn, StartRequest{Cmd: "x"})
			if err == nil || !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("err = %v, want %q", err, tt.wantMsg)
			}
			if strings.Contains(err.Error(), testDaemonToken) {
				t.Fatalf("error leaks the daemon token: %v", err)
			}
		})
	}
}

func TestNextStreamCutOff(t *testing.T) {
	t.Parallel()

	e, conn := newTestEnvd(t, func(w http.ResponseWriter, _ *http.Request) {
		writeFrames(w, frame(0, `{"event":{"start":{"pid":1}}}`))
	})
	p, err := e.Start(context.Background(), conn, StartRequest{Cmd: "sleep"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer p.Close()
	if _, err := p.Next(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err = %v, want unexpected EOF", err)
	}
}

func TestRejectsOversizedFrame(t *testing.T) {
	t.Parallel()

	e, conn := newTestEnvd(t, func(w http.ResponseWriter, _ *http.Request) {
		hdr := make([]byte, 5)
		binary.BigEndian.PutUint32(hdr[1:], maxFrameSize+1)
		_, _ = w.Write(hdr)
	})
	if _, err := e.Start(context.Background(), conn, StartRequest{Cmd: "x"}); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("err = %v", err)
	}
}

type unaryCall struct {
	path string
	body map[string]any
}

func must(t *testing.T, what string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

func TestProcessUnaryCalls(t *testing.T) {
	t.Parallel()

	var calls []unaryCall
	e, conn := newTestEnvd(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/process.Process/Start" {
			writeFrames(w, frame(0, `{"event":{"start":{"pid":42}}}`), frame(0, `{"event":{"end":{}}}`))
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		calls = append(calls, unaryCall{r.URL.Path, body})
		_, _ = w.Write([]byte(`{}`))
	})
	ctx := context.Background()

	plain, err := e.Start(ctx, conn, StartRequest{Cmd: "cat", Stdin: true})
	must(t, "start", err)
	defer plain.Close()
	must(t, "send input", plain.SendInput(ctx, []byte("hi")))
	must(t, "close stdin", plain.CloseStdin(ctx))

	term, err := e.Start(ctx, conn, StartRequest{Cmd: "bash", PTY: true, Rows: 24, Cols: 80})
	must(t, "start pty", err)
	defer term.Close()
	must(t, "send pty input", term.SendInput(ctx, []byte("ls\n")))
	must(t, "close pty stdin", term.CloseStdin(ctx))
	must(t, "resize", term.Resize(ctx, 40, 120))

	assertUnaryCalls(t, calls)
}

func assertUnaryCalls(t *testing.T, calls []unaryCall) {
	t.Helper()
	if len(calls) != 4 {
		t.Fatalf("calls = %+v, want input, close, input, update (no close for a terminal)", calls)
	}
	input, _ := calls[0].body["input"].(map[string]any)
	if calls[0].path != "/process.Process/SendInput" || input["stdin"] != b64("hi") {
		t.Fatalf("stdin call = %+v", calls[0])
	}
	if calls[1].path != "/process.Process/CloseStdin" {
		t.Fatalf("close call = %+v", calls[1])
	}
	ptyInput, _ := calls[2].body["input"].(map[string]any)
	if ptyInput["pty"] != b64("ls\n") || ptyInput["stdin"] != nil {
		t.Fatalf("pty call = %+v", calls[2])
	}
	size, _ := calls[3].body["pty"].(map[string]any)["size"].(map[string]any)
	proc, _ := calls[3].body["process"].(map[string]any)
	if calls[3].path != "/process.Process/Update" || size["rows"] != float64(40) || size["cols"] != float64(120) || proc["pid"] != float64(42) {
		t.Fatalf("update call = %+v", calls[3])
	}
}

func TestProcessUnaryError(t *testing.T) {
	t.Parallel()

	e, conn := newTestEnvd(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/process.Process/Start" {
			writeFrames(w, frame(0, `{"event":{"start":{"pid":1}}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"not_found","message":"process not found"}`))
	})
	p, err := e.Start(context.Background(), conn, StartRequest{Cmd: "x", Stdin: true})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer p.Close()
	err = p.SendInput(context.Background(), []byte("x"))
	if api, ok := errors.AsType[*APIError](err); !ok || api.Status != http.StatusNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestUpload(t *testing.T) {
	t.Parallel()

	var gotBody, query, ctype, token string
	e, conn := newTestEnvd(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody, query, ctype, token = string(raw), r.URL.RawQuery, r.Header.Get("Content-Type"), r.Header.Get("X-Access-Token")
		if r.URL.Path != "/files" || r.Method != http.MethodPost {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`[]`))
	})
	if err := e.Upload(context.Background(), conn, "user", "/tmp/a b.tgz", strings.NewReader("archive")); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if gotBody != "archive" || ctype != "application/octet-stream" || token != testDaemonToken {
		t.Fatalf("body = %q type = %q token = %q", gotBody, ctype, token)
	}
	if !strings.Contains(query, "path=%2Ftmp%2Fa+b.tgz") || !strings.Contains(query, "username=user") {
		t.Fatalf("query = %q", query)
	}
}

func TestUploadError(t *testing.T) {
	t.Parallel()

	e, conn := newTestEnvd(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInsufficientStorage)
		_, _ = w.Write([]byte(`{"code":507,"message":"not enough disk space available"}`))
	})
	err := e.Upload(context.Background(), conn, "user", "/tmp/x", strings.NewReader("x"))
	if err == nil || !strings.Contains(err.Error(), "disk space") {
		t.Fatalf("err = %v", err)
	}
}
