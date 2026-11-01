package daytona

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
)

func TestAttachStreamsBothWays(t *testing.T) {
	t.Parallel()
	got := make(chan string, 1)
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/toolbox/sb-1/processes/p1/attach" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+testKey {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte("hello"))
		if _, data, err := conn.ReadMessage(); err == nil {
			got <- string(data)
		}
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}))
	defer srv.Close()

	att, err := newHTTPToolbox(testKey).Attach(t.Context(), Conn{SandboxID: sandboxID, BaseURL: srv.URL}, "p1")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	defer func() { _ = att.Close() }()
	out, err := att.Read()
	if err != nil || string(out) != "hello" {
		t.Fatalf("Read = %q, %v", out, err)
	}
	if err := att.Write([]byte("ls\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if v := <-got; v != "ls\n" {
		t.Fatalf("server got %q", v)
	}
	if _, err := att.Read(); !errors.Is(err, io.EOF) {
		t.Fatalf("Read after close err = %v", err)
	}
}

func TestAttachHandshakeFailureIsAPIError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	_, err := newHTTPToolbox(testKey).Attach(t.Context(), Conn{SandboxID: sandboxID, BaseURL: srv.URL}, "p1")
	api, ok := errors.AsType[*APIError](err)
	if !ok || api.Status != http.StatusUnauthorized {
		t.Fatalf("err = %v", err)
	}
}

func TestAttachRefusesPlainHTTPToRemoteHost(t *testing.T) {
	t.Parallel()
	_, err := newHTTPToolbox(testKey).Attach(t.Context(), Conn{SandboxID: sandboxID, BaseURL: "http://toolbox.daytona.com"}, "p1")
	if err == nil {
		t.Fatal("Attach accepted a plain HTTP remote endpoint")
	}
}
