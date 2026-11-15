package web

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/registry"
)

type safeBuf struct {
	mu  sync.Mutex
	buf strings.Builder
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

func TestListenAndServePrintsLoopbackAndShutsDown(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	stderr := &safeBuf{}
	opened := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- ListenAndServe(ctx, Options{
			Registry: registry.New(),
			Stderr:   stderr,
			Open: func(page string) error {
				opened <- page
				return nil
			},
		})
	}()

	var page string
	select {
	case page = <-opened:
	case <-time.After(5 * time.Second):
		t.Fatal("browser was not opened")
	}
	if !strings.HasPrefix(page, "http://127.0.0.1:") || strings.Contains(page, "0.0.0.0") {
		t.Fatalf("page = %s", page)
	}
	if !strings.Contains(stderr.String(), "kaxa ui listening on "+page) {
		t.Fatalf("stderr = %s", stderr.String())
	}
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("shutdown = %v", err)
	}
}

func TestListenWarnsWhenTheBrowserFails(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stderr := &safeBuf{}
	errCh := make(chan error, 1)
	go func() {
		errCh <- ListenAndServe(ctx, Options{
			Registry: registry.New(),
			Stderr:   stderr,
			Open: func(string) error {
				cancel()
				return stringError("browser missing")
			},
		})
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("shutdown = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop")
	}
	if !strings.Contains(stderr.String(), "warning: open browser: browser missing") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func TestListenRejectsABusyPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	tcp, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("addr = %T", ln.Addr())
	}
	err = ListenAndServe(t.Context(), Options{Registry: registry.New(), Port: tcp.Port})
	if err == nil || !strings.Contains(err.Error(), "could not listen") {
		t.Fatalf("err = %v", err)
	}
}
