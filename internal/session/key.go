package session

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const keyLen = 32

// loadOrCreateKey returns the HMAC key in dir/key, creating it if there is none.
//
// The key is written to a temporary file first and put in place with a hard link, which fails
// if the key already exists: the first writer wins and nobody ever reads a half-written key.
// (Two srwr mcp started together once did, and failed to start.) It is safe without the session
// lock, though callers hold it anyway.
func loadOrCreateKey(dir string) ([]byte, error) {
	path := filepath.Join(dir, "key")
	key, err := readKey(path)
	if err == nil || !errors.Is(err, fs.ErrNotExist) {
		return key, err
	}

	tmp, err := os.CreateTemp(dir, "key-*.tmp") // 0600
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	buf := make([]byte, keyLen)
	if _, err := rand.Read(buf); err != nil {
		_ = tmp.Close()
		return nil, err
	}
	if _, err := tmp.Write(buf); err != nil {
		_ = tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}

	if err := os.Link(tmp.Name(), path); err != nil && !errors.Is(err, fs.ErrExist) {
		// No hard links on this file system. Everything that writes the key holds the session
		// lock, so a rename is still safe as long as nobody got there first.
		if _, statErr := os.Stat(path); statErr != nil {
			if err := os.Rename(tmp.Name(), path); err != nil {
				return nil, err
			}
		}
	}
	return readKey(path)
}

func readKey(path string) ([]byte, error) {
	key, err := os.ReadFile(path) //nolint:gosec // the key path is built from the workspace
	if err != nil {
		return nil, err
	}
	if len(key) != keyLen {
		return nil, fmt.Errorf("%s has %d bytes, want %d; delete it to make a new key (tokens issued so far stop working)", path, len(key), keyLen)
	}
	return key, nil
}
