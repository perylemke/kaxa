// Package web serves the local kaxa UI on 127.0.0.1.
package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/perylemke/kaxa/internal/config"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	headerSession = "X-Kaxa-Session"
	headerCSRF    = "X-Kaxa-CSRF"
	readHeaderTTL = 5 * time.Second
	idleTTL       = 2 * time.Minute
	shutdownTTL   = 5 * time.Second
)

// Options configures the local UI server.
type Options struct {
	// Registry is the set of sandbox providers.
	Registry *registry.Registry
	// Config holds credentials. They stay in this process.
	Config config.Config
	// Provider limits the page to one provider. Empty lists every provider.
	Provider string
	// Port is the TCP port on 127.0.0.1. Zero picks a free port.
	Port int
	// Stderr receives the listen address and create progress.
	Stderr io.Writer
	// Open is called with the local URL. Nil skips the browser.
	Open func(string) error
	// Home is the root the repository picker browses. Empty uses the user's home directory.
	Home string
	// WorkDir is where the picker starts. Empty uses the working directory.
	WorkDir string
}

// Server is the local UI. It binds only to 127.0.0.1.
type Server struct {
	reg        *registry.Registry
	cfg        config.Config
	only       string
	stderr     io.Writer
	host       string
	session    []byte
	csrf       []byte
	sessionHex string
	csrfHex    string
	secrets    []string
	now        func() time.Time
	home       string
	workDir    string
}

// listen opens the TCP listener; tests replace it to reach the failure paths.
var listen = net.Listen

// New builds a server with a fresh session token and CSRF token.
func New(opts Options) (*Server, error) {
	if opts.Registry == nil {
		return nil, errors.New("ui server requires a registry")
	}
	if opts.Port < 0 || opts.Port > 65535 {
		return nil, usererr.New("invalid port", "use a port from 0 to 65535; 0 picks a free port")
	}
	session, sessionHex, err := newToken()
	if err != nil {
		return nil, err
	}
	csrf, csrfHex, err := newToken()
	if err != nil {
		return nil, err
	}
	return &Server{
		reg:        opts.Registry,
		cfg:        opts.Config,
		only:       opts.Provider,
		stderr:     opts.Stderr,
		session:    session,
		csrf:       csrf,
		sessionHex: sessionHex,
		csrfHex:    csrfHex,
		secrets:    credentialSecrets(opts.Config, sessionHex, csrfHex),
		home:       resolveHome(opts.Home),
		workDir:    workDir(opts.WorkDir),
	}, nil
}

func workDir(dir string) string {
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return dir
}

// resolveHome returns the browsing root with symlinks followed, or "" when it is unknown.
func resolveHome(home string) string {
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if home == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(home); err == nil {
		return resolved
	}
	return home
}

// ListenAndServe listens on 127.0.0.1 until ctx is cancelled.
//
// WriteTimeout is unset because creating a sandbox can take minutes before the response is written.
func ListenAndServe(ctx context.Context, opts Options) error {
	srv, err := New(opts)
	if err != nil {
		return err
	}
	ln, err := listen("tcp", fmt.Sprintf("127.0.0.1:%d", opts.Port))
	if err != nil {
		return usererr.New(
			fmt.Sprintf("could not listen on 127.0.0.1:%d", opts.Port),
			"choose another port with --port",
		)
	}
	host, err := loopbackHost(ln.Addr())
	if err != nil {
		_ = ln.Close()
		return err
	}
	srv.host = host
	// new keeps this file off the Go server signal in project.IsAPI.
	// kaxa ui is a local page, not the repository's HTTP API.
	httpSrv := new(http.Server)
	httpSrv.Handler = srv.Handler()
	httpSrv.ReadHeaderTimeout = readHeaderTTL
	httpSrv.IdleTimeout = idleTTL
	errCh := make(chan error, 1)
	go func() {
		errCh <- httpSrv.Serve(ln)
	}()
	page := "http://" + host + "/"
	srv.log("kaxa ui listening on " + page)
	if opts.Open != nil {
		if openErr := opts.Open(page); openErr != nil {
			srv.event(levelWarn, "ui", "", "open browser: "+openErr.Error())
		}
	}
	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTTL)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
		return ctx.Err()
	case serveErr := <-errCh:
		// Serve only returns http.ErrServerClosed after Shutdown, which runs in the other case.
		return serveErr
	}
}

func loopbackHost(addr net.Addr) (string, error) {
	tcp, ok := addr.(*net.TCPAddr)
	if !ok || tcp == nil || !tcp.IP.Equal(net.IPv4(127, 0, 0, 1)) {
		return "", usererr.New(
			"refusing to listen outside 127.0.0.1",
			"kaxa ui only binds to 127.0.0.1",
		)
	}
	return fmt.Sprintf("127.0.0.1:%d", tcp.Port), nil
}

// Handler is the page and the JSON API, with host, session, and CSRF checks.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.serveIndex)
	mux.HandleFunc("GET /app.css", s.serveCSS)
	mux.HandleFunc("GET /app.js", s.serveJS)
	mux.HandleFunc("GET /api/providers", s.listProviders)
	mux.HandleFunc("GET /api/dirs", s.listDirs)
	mux.HandleFunc("GET /api/sandboxes", s.listSandboxes)
	mux.HandleFunc("POST /api/sandboxes", s.createSandbox)
	mux.HandleFunc("GET /api/sandboxes/{provider}/{name}", s.inspectSandbox)
	mux.HandleFunc("POST /api/sandboxes/{provider}/{name}/restart", s.restartSandbox)
	mux.HandleFunc("POST /api/sandboxes/{provider}/{name}/upgrade", s.upgradeSandbox)
	mux.HandleFunc("POST /api/sandboxes/{provider}/{name}/url-auth", s.urlAuth)
	mux.HandleFunc("POST /api/sandboxes/{provider}/{name}/destroy", s.destroySandbox)
	return s.protect(mux)
}

func (s *Server) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w)
		if r.Host != s.host || s.host == "" {
			s.reject(w, http.StatusForbidden, "rejected host", "open the address printed by kaxa ui")
			return
		}
		if !isAPI(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if !matchToken(r.Header.Get(headerSession), s.session) {
			s.reject(w, http.StatusUnauthorized, "missing session", "reload the page printed by kaxa ui")
			return
		}
		if changesState(r.Method) && !matchToken(r.Header.Get(headerCSRF), s.csrf) {
			s.reject(w, http.StatusForbidden, "missing csrf token", "reload the page printed by kaxa ui")
			return
		}
		started := s.clock()
		rec := &recorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		s.audit(r, rec, started)
	})
}

// reject answers a request that failed the host, session, or CSRF check and logs it.
// The log never repeats the Host header or the path, which the caller controls.
func (s *Server) reject(w http.ResponseWriter, status int, message, hint string) {
	s.event(levelWarn, "request", "", message)
	s.writeStatus(w, status, message, hint)
}

func isAPI(path string) bool {
	return path == "/api" || strings.HasPrefix(path, "/api/")
}

func changesState(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}
