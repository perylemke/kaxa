package web

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/perylemke/kaxa/internal/usererr"
)

// maxDirEntries caps one listing so a huge folder cannot flood the page.
const maxDirEntries = 500

// absPath makes a path absolute; tests replace it to reach the failure path.
var absPath = filepath.Abs

var (
	// errOutsideHome is returned for a path that leaves the user's home directory.
	errOutsideHome = errors.New("outside your home directory")
	errDirMissing  = errors.New("directory not found")
)

type dirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Repo bool   `json:"repo"`
}

type dirListing struct {
	Path      string     `json:"path"`
	Parent    string     `json:"parent"`
	Home      string     `json:"home"`
	Repo      bool       `json:"repo"`
	Entries   []dirEntry `json:"entries"`
	Truncated bool       `json:"truncated"`
}

// resolve returns the absolute path with symlinks followed, and fails when it is outside home.
func (s *Server) resolve(path string) (string, error) {
	if s.home == "" {
		return "", errOutsideHome
	}
	abs, err := absPath(path)
	if err != nil {
		return "", errDirMissing
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errDirMissing
		}
		return "", err
	}
	rel, err := filepath.Rel(s.home, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errOutsideHome
	}
	return resolved, nil
}

// start is where an empty path begins: the working directory when it is inside home, else home.
func (s *Server) start() string {
	if s.workDir != "" {
		if resolved, err := s.resolve(s.workDir); err == nil {
			return resolved
		}
	}
	return s.home
}

func isRepoDir(dir string) bool {
	//gosec:disable G703 -- dir comes from resolve, which keeps it inside home
	_, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil
}

func (s *Server) listDirs(w http.ResponseWriter, r *http.Request) {
	if s.home == "" {
		s.writeErr(w, errors.New("home directory is unknown"))
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		path = s.start()
	}
	dir, err := s.resolve(path)
	if err != nil {
		s.writeDirErr(w, err)
		return
	}
	//gosec:disable G703 -- dir comes from resolve, which keeps it inside home
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		s.writeStatus(w, http.StatusBadRequest, "not a directory", "choose a folder")
		return
	}
	entries, truncated, err := s.readEntries(dir)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	parent := ""
	if dir != s.home {
		parent = filepath.Dir(dir)
	}
	s.writeJSON(w, http.StatusOK, dirListing{
		Path: dir, Parent: parent, Home: s.home, Repo: isRepoDir(dir),
		Entries: entries, Truncated: truncated,
	})
}

func (s *Server) writeDirErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errOutsideHome):
		noteDetail(w, err.Error())
		s.writeStatus(w, http.StatusForbidden, err.Error(), "kaxa ui only browses your home directory")
	case errors.Is(err, errDirMissing):
		noteDetail(w, err.Error())
		s.writeStatus(w, http.StatusNotFound, err.Error(), "")
	default:
		s.writeErr(w, err)
	}
}

// readEntries lists the visible sub-folders of dir that stay inside home.
func (s *Server) readEntries(dir string) ([]dirEntry, bool, error) {
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil, false, usererr.New("cannot read this folder", "check its permissions")
	}
	out := make([]dirEntry, 0, len(items))
	for _, item := range items {
		if strings.HasPrefix(item.Name(), ".") {
			continue
		}
		resolved, err := s.resolve(filepath.Join(dir, item.Name()))
		if err != nil {
			continue
		}
		//gosec:disable G703 -- resolved comes from resolve, which keeps it inside home
		if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
			continue
		}
		if len(out) == maxDirEntries {
			return out, true, nil
		}
		out = append(out, dirEntry{Name: item.Name(), Path: resolved, Repo: isRepoDir(resolved)})
	}
	return out, false, nil
}

// checkDirs refuses a comma-separated list of paths when one is outside home.
// A path that does not exist passes, so the repository check can report it.
func (s *Server) checkDirs(raw string) error {
	for part := range strings.SplitSeq(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, err := s.resolve(part); err != nil && !errors.Is(err, errDirMissing) {
			return err
		}
	}
	return nil
}
