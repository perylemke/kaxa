//go:build unix

package sandbox

import (
	"os"
	"syscall"
)

// openEnvFile opens path for reading and fails when path is a symlink.
// The caller already rejected a symlink; O_NOFOLLOW closes the race where the
// path changes between that check and the open.
func openEnvFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0) //gosec:disable G304 -- the caller names the file; a symlink was already rejected
}
