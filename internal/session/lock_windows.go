//go:build windows

package session

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

// lockfileExclusiveLock is LOCKFILE_EXCLUSIVE_LOCK. Without LOCKFILE_FAIL_IMMEDIATELY the call waits.
const lockfileExclusiveLock = 2

// lockFile takes an exclusive lock on the first byte of the file at path, waiting as long as it
// takes. The returned function releases it. It calls kernel32 directly, so there is no cgo and no
// dependency outside the standard library.
func lockFile(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // the lock path is built from the workspace
	if err != nil {
		return nil, err
	}
	h := f.Fd()
	var ol syscall.Overlapped
	r, _, callErr := procLockFileEx.Call(h, lockfileExclusiveLock, 0, 1, 0, uintptr(unsafe.Pointer(&ol))) //nolint:gosec // the syscall takes a pointer to the OVERLAPPED structure
	if r == 0 {
		_ = f.Close()
		return nil, callErr
	}
	return func() {
		var ol syscall.Overlapped
		_, _, _ = procUnlockFileEx.Call(h, 0, 1, 0, uintptr(unsafe.Pointer(&ol))) //nolint:gosec // the syscall takes a pointer to the OVERLAPPED structure
		_ = f.Close()
	}, nil
}
