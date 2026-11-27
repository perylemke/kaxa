package vercel

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func rawClient(t *testing.T, handler http.HandlerFunc) *restClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return newRESTClient(srv.URL, testToken, "proj", "")
}

func TestUserErrMessages(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		err  error
		want string
	}{
		{"unauthorized", &APIError{Status: 401, Message: "bad " + testToken}, "rejected"},
		{"forbidden", &APIError{Status: 403}, "rejected"},
		{"plan", &APIError{Status: 402}, "plan or billing"},
		{"rate", &APIError{Status: 429}, "rate limit"},
		{"already", ErrAlreadyExists, "already exists"},
		{"missing", ErrNotFound, "not found"},
		{"other", &APIError{Status: 500, Message: "oops " + testToken}, "status 500"},
		{"plain", errors.New("dial " + testToken), "dial"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := userErr(tt.err, "box", testToken)
			if got == nil || !strings.Contains(got.Error(), tt.want) {
				t.Fatalf("userErr() = %v, want %q", got, tt.want)
			}
			if strings.Contains(got.Error(), testToken) {
				t.Fatalf("token leaked: %v", got)
			}
		})
	}
	if userErr(nil, "box") != nil {
		t.Fatal("userErr(nil) != nil")
	}
	if got := (&APIError{Status: 500}).Error(); got != "vercel: API returned status 500" {
		t.Fatalf("Error() = %q", got)
	}
}

func TestAPIErrorBodies(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ body, want string }{
		{`{"error":{"code":"x","message":"nope"}}`, "nope"},
		{"plain text", "plain text"},
		{"", ""},
	} {
		c := rawClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, tt.body)
		})
		_, err := c.Get(context.Background(), "x", false)
		var api *APIError
		if !errors.As(err, &api) || api.Status != 400 || api.Message != tt.want {
			t.Errorf("body %q: error = %v", tt.body, err)
		}
	}
}

func TestClientBadResponses(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ok := func(body string) *restClient {
		return rawClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) })
	}
	for name, call := range map[string]func(c *restClient) error{
		"decode": func(c *restClient) error { _, err := c.Get(ctx, "x", false); return err },
		"list decode": func(c *restClient) error {
			_, err := c.List(ctx)
			return err
		},
		"create unmanaged": func(c *restClient) error { _, err := c.Create(ctx, CreateParams{Name: "x"}); return err },
		"start no id":      func(c *restClient) error { _, err := c.Start(ctx, "s", CmdRequest{Command: "x"}); return err },
		"snapshot no id":   func(c *restClient) error { _, err := c.CreateSnapshot(ctx, "s"); return err },
		"snapshots decode": func(c *restClient) error { _, err := c.ListSnapshots(ctx, "x"); return err },
	} {
		body := "not json"
		if strings.Contains(name, "unmanaged") || strings.Contains(name, "no id") {
			body = "{}"
		}
		if err := call(ok(body)); err == nil {
			t.Errorf("%s: bad response accepted", name)
		}
	}
}

func TestClientNotFoundMapping(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := rawClient(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	for name, err := range map[string]error{
		"get":       func() error { _, err := c.Get(ctx, "x", true); return err }(),
		"delete":    c.Delete(ctx, "x"),
		"update":    c.Update(ctx, "x", map[string]any{}),
		"stop":      c.StopSession(ctx, "s"),
		"extend":    c.ExtendTimeout(ctx, "s", 1),
		"mkdir":     c.Mkdir(ctx, "s", "/d"),
		"tar":       c.WriteTar(ctx, "s", "/d", strings.NewReader("x")),
		"stdin":     c.WriteStdin(ctx, "s", "c", []byte("x"), 0, false),
		"kill":      c.Kill(ctx, "s", "c"),
		"delsnap":   c.DeleteSnapshot(ctx, "x"),
		"run":       func() error { _, err := c.Run(ctx, "s", CmdRequest{Command: "x"}); return err }(),
		"start":     func() error { _, err := c.Start(ctx, "s", CmdRequest{Command: "x"}); return err }(),
		"snapshot":  func() error { _, err := c.CreateSnapshot(ctx, "s"); return err }(),
		"listsnaps": func() error { _, err := c.ListSnapshots(ctx, "x"); return err }(),
	} {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: error = %v, want ErrNotFound", name, err)
		}
	}
}

func TestClientTransportErrors(t *testing.T) {
	t.Parallel()
	c := newRESTClient("http://127.0.0.1:1", testToken, "proj", "")
	ctx := context.Background()
	if _, err := c.Get(ctx, "x", false); err == nil {
		t.Fatal("Get() succeeded without a server")
	}
	if err := c.WriteTar(ctx, "s", "/d", strings.NewReader("x")); err == nil {
		t.Fatal("WriteTar() succeeded without a server")
	}
	if _, err := c.Run(ctx, "s", CmdRequest{Command: "x"}); err == nil {
		t.Fatal("Run() succeeded without a server")
	}
	bad := newRESTClient("http://[::1", testToken, "proj", "")
	if _, err := bad.Get(ctx, "x", false); err == nil {
		t.Fatal("Get() accepted a bad URL")
	}
	if err := bad.WriteTar(ctx, "s", "/d", strings.NewReader("x")); err == nil {
		t.Fatal("WriteTar() accepted a bad URL")
	}
	if _, err := bad.Run(ctx, "s", CmdRequest{Command: "x"}); err == nil {
		t.Fatal("Run() accepted a bad URL")
	}
}

func TestCmdBodyDefaults(t *testing.T) {
	t.Parallel()
	body := cmdBody(CmdRequest{Command: "ls"})
	if args, ok := body["args"].([]string); !ok || args == nil {
		t.Fatalf("args = %#v, want an empty list", body["args"])
	}
}

//nolint:gocyclo // one scenario that checks many fields of a single response
func TestStreamDecoding(t *testing.T) {
	t.Parallel()
	enc := func(items ...[]byte) io.ReadCloser { return io.NopCloser(bytes.NewReader(bytes.Join(items, nil))) }
	raw := func(v any) []byte {
		b, err := cbor.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, tt := range []struct {
		name    string
		body    io.ReadCloser
		wantErr string
	}{
		{"truncated", enc(raw([]any{1, []byte("x")})[:2]), "decode command stream"},
		{"wrong arity", enc(raw([]any{1})), "1 fields"},
		{"bad kind", enc(raw([]any{"x", 1})), "decode command stream"},
		{"bad output", enc(item(itemStdout, "text")), "decode command output"},
		{"bad failure", enc(item(itemFailure, "text")), "decode command failure"},
		{"bad command", enc(item(itemCommand, "text")), "decode command status"},
		{"empty", enc(), "unexpected EOF"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := newCBORStream(tt.body)
			_, err := s.Next()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Next() error = %v, want %q", err, tt.wantErr)
			}
			_ = s.Close()
		})
	}
	t.Run("unknown kind is skipped and end is final", func(t *testing.T) {
		t.Parallel()
		code := 0
		s := newCBORStream(enc(item(77, "x"), item(itemCommand, map[string]any{"id": "c", "exitCode": code})))
		ev, err := s.Next()
		if err != nil || ev.Kind != 0 {
			t.Fatalf("unknown item = %+v, %v", ev, err)
		}
		ev, err = s.Next()
		if err != nil || ev.Kind != EventEnd || ev.ExitCode == nil || *ev.ExitCode != 0 {
			t.Fatalf("end = %+v, %v", ev, err)
		}
		if _, err := s.Next(); !errors.Is(err, io.EOF) {
			t.Fatalf("after end error = %v, want EOF", err)
		}
	})
	t.Run("a second status without an exit code ends the command", func(t *testing.T) {
		t.Parallel()
		s := newCBORStream(enc(item(itemCommand, map[string]any{"id": "c"}), item(itemCommand, map[string]any{"id": "c"})))
		if ev, _ := s.Next(); ev.Kind != EventStarted || ev.CmdID != "c" {
			t.Fatalf("start = %+v", ev)
		}
		if ev, _ := s.Next(); ev.Kind != EventEnd || ev.ExitCode != nil {
			t.Fatalf("end = %+v", ev)
		}
	})
}

func TestDrainSkipsUnknownEvents(t *testing.T) {
	t.Parallel()
	code := 0
	s := &session{rec: Record{Name: boxName}}
	err := s.drain(context.Background(), &scripted{events: []Event{{}, {Kind: EventStdout, Data: []byte("x")}, {Kind: EventEnd, ExitCode: &code}}}, runSpec{}, false)
	if err != nil {
		t.Fatal(err)
	}
}

type scripted struct{ events []Event }

func (s *scripted) Next() (Event, error) {
	ev := s.events[0]
	s.events = s.events[1:]
	return ev, nil
}

func (s *scripted) Close() error { return nil }

func TestInspectFieldsEmpty(t *testing.T) {
	t.Parallel()
	if fields := vercelDetailFields(Record{}, "", ""); len(fields) != 0 {
		t.Fatalf("fields = %+v", fields)
	}
	if got := toolPath(); !strings.HasPrefix(got, vercelHome) {
		t.Fatalf("toolPath() = %q", got)
	}
	var _ sandbox.Provider = New()
}

func TestResumeWaitsLongerThanOtherCalls(t *testing.T) {
	t.Parallel()
	slow := func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(150 * time.Millisecond)
		_, _ = io.WriteString(w, `{"sandbox":{"name":"box","status":"running","tags":{"kaxa-managed":"true"}}}`)
	}
	c := rawClient(t, slow)
	c.timeout, c.resumeTimeout = 50*time.Millisecond, 2*time.Second

	if _, err := c.Get(context.Background(), boxName, true); err != nil {
		t.Fatalf("resume = %v, want success within the resume timeout", err)
	}
	if _, err := c.Get(context.Background(), boxName, false); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("plain get = %v, want a deadline error", err)
	}
}

func TestUserErrTimeoutHidesURL(t *testing.T) {
	t.Parallel()
	c := rawClient(t, func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	c.timeout, c.resumeTimeout = 30*time.Millisecond, 30*time.Millisecond
	_, err := c.Get(context.Background(), boxName, true)
	got := userErr(err, boxName, testToken)
	if got == nil || !strings.Contains(got.Error(), "still starting") {
		t.Fatalf("userErr() = %v, want the still-starting message", got)
	}
	if strings.Contains(got.Error(), "http") || strings.Contains(got.Error(), "proj") {
		t.Fatalf("message shows the request URL: %v", got)
	}
}
