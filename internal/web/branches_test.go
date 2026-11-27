package web

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/perylemke/kaxa/internal/registry"
)

// The tests below replace package variables, so none of them runs in parallel.

func TestTokenFailures(t *testing.T) {
	failing := errors.New("no entropy")
	for name, failAt := range map[string]int{"session": 1, "csrf": 2} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			prev := randRead
			randRead = func(b []byte) (int, error) {
				calls++
				if calls == failAt {
					return 0, failing
				}
				return prev(b)
			}
			t.Cleanup(func() { randRead = prev })
			if _, err := New(Options{Registry: registry.New()}); !errors.Is(err, failing) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestResolveRefusesWhenTheWorkingDirectoryIsGone(t *testing.T) {
	home, _ := pickerHome(t)
	s, _ := newServer(t, nil, Options{Home: home})
	prev := absPath
	absPath = func(string) (string, error) { return "", errors.New("no working directory") }
	t.Cleanup(func() { absPath = prev })
	if _, err := s.resolve("relative"); !errors.Is(err, errDirMissing) {
		t.Fatalf("err = %v", err)
	}
}

func TestHomeIsUnknownWithoutHOME(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("home", "")
	if got := resolveHome(""); got != "" {
		t.Skipf("this platform finds a home without HOME: %q", got)
	}
}

func TestDirsThroughAFileIsAServerError(t *testing.T) {
	t.Parallel()

	home, _ := pickerHome(t)
	s, h := newServer(t, nil, Options{Home: home})
	if rec := getDirs(t, h, s, filepath.Join(home, "note.txt", "child")); rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
}

func TestCheckDirsSkipsEmptyParts(t *testing.T) {
	t.Parallel()

	home, repo := pickerHome(t)
	s, _ := newServer(t, nil, Options{Home: home})
	if err := s.checkDirs(repo + ", ," + filepath.Join(home, "missing")); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestMalformedRequestsAreRefused(t *testing.T) {
	t.Parallel()

	fp := &fakeProvider{name: testProvider}
	s, h := newServer(t, capable{fp}, Options{})
	cases := []struct {
		name   string
		method string
		path   string
		body   any
		want   int
	}{
		{name: "create body", method: http.MethodPost, path: "/api/sandboxes", body: "nope", want: http.StatusBadRequest},
		{name: "restart path", method: http.MethodPost, path: "/api/sandboxes/LAB/box/restart", want: http.StatusBadRequest},
		{name: "url auth path", method: http.MethodPost, path: "/api/sandboxes/LAB/box/url-auth", body: map[string]string{"auth": "public"}, want: http.StatusBadRequest},
		{name: "url auth body", method: http.MethodPost, path: "/api/sandboxes/lab/box/url-auth", body: []string{"x"}, want: http.StatusBadRequest},
		{name: "destroy path", method: http.MethodPost, path: "/api/sandboxes/LAB/box/destroy", body: map[string]string{"confirm": "box"}, want: http.StatusBadRequest},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if rec := do(t, h, s, tt.method, tt.path, tt.body); rec.Code != tt.want {
				t.Fatalf("status = %d %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestListWithAnUnknownOnlyProvider(t *testing.T) {
	t.Parallel()

	s, h := newServer(t, nil, Options{Registry: registry.New(), Provider: "missing"})
	if rec := do(t, h, s, http.MethodGet, "/api/sandboxes", nil); rec.Code == http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
}

// oddListener reports an address and an accept error of the test's choosing.
type oddListener struct {
	net.Listener
	addr      net.Addr
	acceptErr error
}

func (l oddListener) Addr() net.Addr { return l.addr }

func (l oddListener) Accept() (net.Conn, error) { return nil, l.acceptErr }

func withListener(t *testing.T, addr net.Addr, acceptErr error) {
	t.Helper()
	prev := listen
	listen = func(string, string) (net.Listener, error) {
		inner, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		return oddListener{Listener: inner, addr: addr, acceptErr: acceptErr}, nil
	}
	t.Cleanup(func() { listen = prev })
}

func TestListenAndServeRefusesANonLoopbackListener(t *testing.T) {
	withListener(t, &net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 80}, errors.New("unused"))
	if err := ListenAndServe(t.Context(), Options{Registry: registry.New()}); err == nil {
		t.Fatal("a listener outside 127.0.0.1 must be refused")
	}
}

func TestListenAndServeReturnsAServeError(t *testing.T) {
	failing := errors.New("accept failed")
	withListener(t, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4000}, failing)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := ListenAndServe(ctx, Options{Registry: registry.New()}); !errors.Is(err, failing) {
		t.Fatalf("err = %v", err)
	}
}

func TestNewKeepsTheGivenWorkDir(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(os.TempDir(), "kaxa-work")
	if got := workDir(dir); got != dir {
		t.Fatalf("workDir = %q", got)
	}
}
