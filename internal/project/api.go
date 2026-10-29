package project

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const apiFileReadLimit = 1 << 20

var (
	goAPIModules = []string{
		"github.com/gin-gonic/gin",
		"github.com/labstack/echo",
		"github.com/go-chi/chi",
		"github.com/gofiber/fiber",
		"github.com/gorilla/mux",
	}
	pythonAPIModules = []string{
		"fastapi",
		"flask",
		"django",
		"starlette",
		"litestar",
		"sanic",
		"uvicorn",
	}
	nodeAPIModules = []string{
		"express",
		"fastify",
		"@nestjs/core",
		"hono",
		"koa",
		"@hapi/hapi",
		"restify",
	}
	rustAPIModules = []string{
		"axum",
		"actix-web",
		"rocket",
		"warp",
		"poem",
		"salvo",
	}
	rubyAPIModules = []string{
		"rails",
		"sinatra",
		"roda",
		"hanami",
	}
	clojureAPIModules = []string{
		"ring/ring",
		"ring/ring-core",
		"ring/ring-jetty-adapter",
		"compojure/compojure",
		"metosin/reitit",
		"io.pedestal/pedestal",
		"http-kit/http-kit",
		"aleph/aleph",
	}
)

func goServerSignals() []string {
	listen := "http.ListenAndServe"
	server := "http.Server"
	return []string{listen + "TLS(", listen + "(", server + "{"}
}

// IsAPI reports whether root looks like an HTTP API.
// One manifest dependency or Go server signal is enough.
// Vendor trees, virtualenvs, and Go test files are ignored.
func IsAPI(root string) (bool, error) {
	ok, err := manifestIsAPI(root)
	if err != nil || ok {
		return ok, err
	}
	return goListenAPI(root)
}

func manifestIsAPI(root string) (bool, error) {
	checks := []func(string) (bool, error){
		goModAPI,
		pythonAPI,
		nodeAPI,
		rustAPI,
		rubyAPI,
		clojureAPI,
	}
	for _, check := range checks {
		ok, err := check(root)
		if err != nil || ok {
			return ok, err
		}
	}
	return false, nil
}

func goModAPI(root string) (bool, error) {
	data, ok, err := readOptional(root, "go.mod")
	if err != nil || !ok {
		return false, err
	}
	return containsAny(data, goAPIModules), nil
}

func pythonAPI(root string) (bool, error) {
	for _, name := range []string{"pyproject.toml", "requirements.txt", "uv.lock"} {
		data, ok, err := readOptional(root, name)
		if err != nil {
			return false, err
		}
		if ok && containsAny(data, pythonAPIModules) {
			return true, nil
		}
	}
	return false, nil
}

func nodeAPI(root string) (bool, error) {
	data, ok, err := readOptional(root, "package.json")
	if err != nil || !ok {
		return false, err
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false, fmt.Errorf("parse package.json: %w", err)
	}
	return depsHaveAPI(pkg.Dependencies) || depsHaveAPI(pkg.DevDependencies), nil
}

func depsHaveAPI(deps map[string]string) bool {
	for _, name := range nodeAPIModules {
		if _, ok := deps[name]; ok {
			return true
		}
	}
	return false
}

func rustAPI(root string) (bool, error) {
	data, ok, err := readOptional(root, "Cargo.toml")
	if err != nil || !ok {
		return false, err
	}
	return containsAny(data, rustAPIModules), nil
}

func rubyAPI(root string) (bool, error) {
	for _, name := range []string{"Gemfile", "Gemfile.lock"} {
		data, ok, err := readOptional(root, name)
		if err != nil {
			return false, err
		}
		if ok && containsAny(data, rubyAPIModules) {
			return true, nil
		}
	}
	return false, nil
}

func clojureAPI(root string) (bool, error) {
	data, ok, err := readOptional(root, "deps.edn")
	if err != nil || !ok {
		return false, err
	}
	return containsAny(data, clojureAPIModules), nil
}

func goListenAPI(root string) (bool, error) {
	var found bool
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && skipAPIDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() || !isGoSource(entry.Name()) || !withinRoot(root, path) {
			return nil
		}
		ok, err := fileHasSignal(path)
		if err != nil {
			return err
		}
		if ok {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("scan go sources: %w", err)
	}
	return found, nil
}

func skipAPIDir(name string) bool {
	switch name {
	case ".git", "vendor", "node_modules", ".venv", "venv", "target", "dist", "__pycache__":
		return true
	default:
		return false
	}
}

func isGoSource(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
}

func fileHasSignal(path string) (bool, error) {
	f, err := os.Open(path) //gosec:disable G304 -- path is a regular file under the repository root
	if err != nil {
		return false, fmt.Errorf("read go source: %w", err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, apiFileReadLimit))
	if err != nil {
		return false, fmt.Errorf("read go source: %w", err)
	}
	return containsAnyLeft(data, goServerSignals()), nil
}

// readOptional reads a manifest from the repository root. Only a regular file is
// read, up to apiFileReadLimit, so a symlink to /dev/zero or a huge file cannot
// exhaust memory.
func readOptional(root, name string) ([]byte, bool, error) {
	path := filepath.Join(root, name)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return nil, false, nil
	}
	f, err := os.Open(path) //gosec:disable G304 -- name is a fixed manifest under the repository root
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", name, err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, apiFileReadLimit))
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", name, err)
	}
	return data, true, nil
}

func containsAny(data []byte, names []string) bool {
	return containsTokens(data, names, true)
}

func containsAnyLeft(data []byte, names []string) bool {
	return containsTokens(data, names, false)
}

func containsTokens(data []byte, names []string, rightBound bool) bool {
	text := strings.ToLower(string(data))
	for _, name := range names {
		if containsToken(text, strings.ToLower(name), rightBound) {
			return true
		}
	}
	return false
}

func containsToken(text, name string, rightBound bool) bool {
	from := 0
	for {
		rel := strings.Index(text[from:], name)
		if rel < 0 {
			return false
		}
		start := from + rel
		end := start + len(name)
		if tokenBounded(text, start, end, rightBound) {
			return true
		}
		from = end
	}
}

func tokenBounded(text string, start, end int, rightBound bool) bool {
	if start > 0 && isModuleChar(text[start-1]) {
		return false
	}
	if rightBound && end < len(text) && isModuleChar(text[end]) {
		return false
	}
	return true
}

func isModuleChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_' || b == '-'
}

func withinRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
