package reposync

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// Archive streams a gzip-compressed tar of the repository at root.
// The caller must close the reader. name is the top-level directory inside the archive.
// Tracked files are included even when gitignored. Untracked files honor exclude rules.
// The .git directory is included. Symlinks are stored as symlinks. Submodule directories are omitted.
func Archive(ctx context.Context, root, name string) (io.ReadCloser, error) {
	if err := validateRepoName(name); err != nil {
		return nil, err
	}
	info, err := os.Lstat(filepath.Join(root, ".git"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotRepository
		}
		return nil, fmt.Errorf("check git metadata: %w", err)
	}
	if !info.IsDir() {
		return nil, ErrGitDirFile
	}

	pr, pw := io.Pipe()
	go func() {
		err := writeArchive(ctx, pw, root, name)
		_ = pw.CloseWithError(err)
	}()
	return pr, nil
}

func writeArchive(ctx context.Context, w io.Writer, root, name string) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	if err := addWorkTree(ctx, tw, root, name); err != nil {
		return err
	}
	if err := addGitDir(tw, root, name); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("close archive: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("close archive: %w", err)
	}
	return nil
}

func addWorkTree(ctx context.Context, tw *tar.Writer, root, name string) error {
	files, err := workTreeFiles(ctx, root)
	if err != nil {
		return err
	}
	for _, rel := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := addFSEntry(tw, root, name, rel); err != nil {
			return err
		}
	}
	return nil
}

func workTreeFiles(ctx context.Context, root string) ([]string, error) {
	listed, err := listWorkTree(ctx, root)
	if err != nil {
		return nil, err
	}
	skip := map[string]struct{}{}
	cleaned := make([]string, 0, len(listed))
	for _, rel := range listed {
		rel, err = safeRel(rel)
		if err != nil {
			return nil, err
		}
		info, err := os.Lstat(filepath.Join(root, rel))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("stat %s: %w", rel, err)
		}
		if info.IsDir() {
			skip[rel] = struct{}{}
			continue
		}
		cleaned = append(cleaned, rel)
	}
	files := make([]string, 0, len(cleaned))
	seen := map[string]struct{}{}
	for _, rel := range cleaned {
		if _, ok := seen[rel]; ok || underSkippedDir(rel, skip) {
			continue
		}
		seen[rel] = struct{}{}
		files = append(files, rel)
	}
	return files, nil
}

func underSkippedDir(rel string, skip map[string]struct{}) bool {
	for dir := range skip {
		if rel == dir || strings.HasPrefix(rel, dir+"/") {
			return true
		}
	}
	return false
}

func listWorkTree(ctx context.Context, root string) ([]string, error) {
	//gosec:disable G204 -- repository path is a git argv element, not a shell command
	cmd := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			if msg := strings.TrimSpace(string(exitErr.Stderr)); msg != "" {
				return nil, fmt.Errorf("list repository files: %s", msg)
			}
		}
		return nil, fmt.Errorf("list repository files: %w", err)
	}
	if len(out) == 0 {
		return nil, nil
	}
	parts := bytes.Split(out, []byte{0})
	files := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		files = append(files, string(part))
	}
	return files, nil
}

func addGitDir(tw *tar.Writer, root, name string) error {
	gitRoot := filepath.Join(root, ".git")
	return filepath.WalkDir(gitRoot, func(entryPath string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, entryPath)
		if err != nil {
			return fmt.Errorf("relative path: %w", err)
		}
		rel, err = safeRel(rel)
		if err != nil {
			return err
		}
		return addFSEntry(tw, root, name, rel)
	})
}

func addFSEntry(tw *tar.Writer, root, name, rel string) error {
	abs := filepath.Join(root, rel)
	info, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", rel, err)
	}
	if err := ensureInside(root, abs, info); err != nil {
		return err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return addSymlink(tw, name, rel, abs, info)
	case info.IsDir():
		return addDir(tw, name, rel, info)
	case info.Mode().IsRegular():
		return addRegular(tw, name, rel, abs, info)
	default:
		return nil
	}
}

func ensureInside(root, abs string, info os.FileInfo) error {
	if err := relInside(root, abs); err != nil {
		return err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve repository: %w", err)
	}
	check := abs
	if info.Mode()&os.ModeSymlink != 0 {
		check = filepath.Dir(abs)
	}
	resolved, err := filepath.EvalSymlinks(check)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	return relInside(resolvedRoot, resolved)
}

func relInside(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrPathEscape, target)
	}
	if rel == "." {
		return nil
	}
	if _, err := safeRel(rel); err != nil {
		return err
	}
	return nil
}

func addSymlink(tw *tar.Writer, name, rel, abs string, info os.FileInfo) error {
	target, err := os.Readlink(abs)
	if err != nil {
		return fmt.Errorf("read symlink %s: %w", rel, err)
	}
	hdr, err := newHeader(name, rel, info, tar.TypeSymlink)
	if err != nil {
		return err
	}
	hdr.Linkname = target
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("archive %s: %w", hdr.Name, err)
	}
	return nil
}

func addDir(tw *tar.Writer, name, rel string, info os.FileInfo) error {
	hdr, err := newHeader(name, rel, info, tar.TypeDir)
	if err != nil {
		return err
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("archive %s: %w", hdr.Name, err)
	}
	return nil
}

func addRegular(tw *tar.Writer, name, rel, abs string, info os.FileInfo) (err error) {
	hdr, err := newHeader(name, rel, info, tar.TypeReg)
	if err != nil {
		return err
	}
	//gosec:disable G304 -- path is confined to the repository root
	f, err := os.Open(abs)
	if err != nil {
		return fmt.Errorf("open %s: %w", rel, err)
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	if err = tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("archive %s: %w", hdr.Name, err)
	}
	if _, err = io.Copy(tw, f); err != nil {
		return fmt.Errorf("archive %s: %w", hdr.Name, err)
	}
	return nil
}

func newHeader(prefix, rel string, info os.FileInfo, typeflag byte) (*tar.Header, error) {
	hdrName, err := headerName(prefix, rel, typeflag == tar.TypeDir)
	if err != nil {
		return nil, err
	}
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0o644
		if typeflag == tar.TypeDir {
			mode = 0o755
		}
	}
	return &tar.Header{
		Name:     hdrName,
		Mode:     int64(mode),
		ModTime:  info.ModTime(),
		Typeflag: typeflag,
		Size:     regularSize(info, typeflag),
	}, nil
}

func regularSize(info os.FileInfo, typeflag byte) int64 {
	if typeflag != tar.TypeReg {
		return 0
	}
	return info.Size()
}

func headerName(prefix, rel string, dir bool) (string, error) {
	name := path.Join(prefix, filepath.ToSlash(rel))
	if !strings.HasPrefix(name, prefix+"/") || strings.Contains(name, "../") || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("%w: %q", ErrPathEscape, name)
	}
	if dir {
		name += "/"
	}
	return name, nil
}
