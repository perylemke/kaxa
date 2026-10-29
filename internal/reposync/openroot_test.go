package reposync

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// A file swapped for a symlink after ensureInside checked it must not be read.
func TestAddRegularRefusesPathThroughSymlinkOutsideRoot(t *testing.T) {
	t.Parallel()

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "id_ed25519"), []byte("PRIVATE KEY"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	root := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "swapped")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	info, err := os.Stat(filepath.Join(outside, "id_ed25519"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	tw := tar.NewWriter(io.Discard)
	err = addRegular(tw, root, "api", "swapped/id_ed25519", info)
	if err == nil {
		t.Fatal("file outside the repository was read")
	}
}

func TestAddRegularReadsFileInsideRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	info, err := os.Stat(filepath.Join(root, "main.go"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	tw := tar.NewWriter(io.Discard)
	if err := addRegular(tw, root, "api", "main.go", info); err != nil {
		t.Fatalf("addRegular: %v", err)
	}
}
