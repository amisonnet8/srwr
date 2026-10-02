//go:build unix

package session

import (
	"errors"
	"os"
	"syscall"
)

// lockFile takes an exclusive flock on the file at path, waiting as long as it takes.
// The returned function releases it.
func lockFile(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // the lock path is built from the workspace
	if err != nil {
		return nil, err
	}
	fd := int(f.Fd())
	for {
		err = syscall.Flock(fd, syscall.LOCK_EX)
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(fd, syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
