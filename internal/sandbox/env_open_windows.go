//go:build windows

package sandbox

import (
	"fmt"
	"os"
	"syscall"

	"github.com/perylemke/kaxa/internal/usererr"
)

const (
	fileAttributeReparsePoint = 0x400
	fileFlagOpenReparsePoint  = 0x200000
)

// openEnvFile opens path for reading and fails when path is a reparse point.
// Windows has no O_NOFOLLOW. FILE_FLAG_OPEN_REPARSE_POINT opens the link
// itself, and the attribute check rejects it.
func openEnvFile(path string) (*os.File, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("env file: %w", err)
	}
	handle, err := syscall.CreateFile(
		name,
		syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL|fileFlagOpenReparsePoint,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("env file: %w", err)
	}
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(handle, &info); err != nil {
		_ = syscall.CloseHandle(handle)
		return nil, fmt.Errorf("env file: %w", err)
	}
	if info.FileAttributes&fileAttributeReparsePoint != 0 {
		_ = syscall.CloseHandle(handle)
		return nil, usererr.New(fmt.Sprintf("env file %s is a symlink", path), "pass the real file")
	}
	return os.NewFile(uintptr(handle), path), nil
}
