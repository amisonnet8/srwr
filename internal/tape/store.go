package tape

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GzSuffix is added to FileSuffix for a tape that was closed: the same JSONL, compressed with gzip.
const GzSuffix = ".gz"

// MaxTapeSize is the most bytes a compressed tape may expand to. A tape can be shared, and a small file must not be able to
// fill the memory of whoever opens it.
const MaxTapeSize = 1 << 30

// Find returns the file of the tape with the given ID in dir. A tape that is still being written is plain JSONL; a closed one is
// compressed. When both exist (a crash between making the compressed file and removing the plain one), the plain one is
// the tape: it was complete first.
func Find(dir, id string) (path string, ok bool) {
	plain := filepath.Join(dir, FileName(id))
	if _, err := os.Stat(plain); err == nil {
		return plain, true
	}
	gz := plain + GzSuffix
	if _, err := os.Stat(gz); err == nil {
		return gz, true
	}
	return "", false
}

// ReadAll reads the whole tape with the given ID, expanding it when it is compressed. A tape that is not there is an error for
// which errors.Is(err, os.ErrNotExist) holds.
func ReadAll(dir, id string) ([]byte, error) {
	path, ok := Find(dir, id)
	if !ok {
		return nil, fmt.Errorf("tape %s: %w", id, os.ErrNotExist)
	}
	return ReadFile(path)
}

// ReadFile reads the tape file at path, expanding it when its name ends in GzSuffix.
func ReadFile(path string) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // the caller built the path from a tape ID that passed ValidID
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	if !strings.HasSuffix(path, GzSuffix) {
		return io.ReadAll(f)
	}
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer func() { _ = zr.Close() }()
	data, err := io.ReadAll(io.LimitReader(zr, MaxTapeSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxTapeSize {
		return nil, fmt.Errorf("the tape expands to more than %d bytes", MaxTapeSize)
	}
	return data, nil
}

// IDs lists the IDs of the tapes in dir, plain and compressed, each once, in increasing order. A name that is not a bare tape ID
// is left out.
func IDs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		id, ok := strings.CutSuffix(strings.TrimSuffix(e.Name(), GzSuffix), FileSuffix)
		if !ok || !ValidID(id) || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// IDOf returns the tape ID that a name or a path stands for: the base name without FileSuffix or FileSuffix+GzSuffix.
func IDOf(nameOrPath string) string {
	base := filepath.Base(nameOrPath)
	base = strings.TrimSuffix(base, GzSuffix)
	return strings.TrimSuffix(base, FileSuffix)
}

// Compress closes the tape with the given ID in dir: it writes the compressed file next to the plain one, gives it the plain
// file's time of last change (so the list of tapes does not reorder), and removes the plain file. The same tape always compresses to the same bytes. A
// tape that is already compressed, or not there, is left as it is.
func Compress(dir, id string) error {
	plain := filepath.Join(dir, FileName(id))
	src, err := os.Open(plain) //nolint:gosec // id passed ValidID
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	info, err := src.Stat()
	if err != nil {
		_ = src.Close()
		return err
	}
	tmp, err := os.CreateTemp(dir, ".compress-*.tmp")
	if err != nil {
		_ = src.Close()
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // gone after the rename; what a failed attempt left otherwise
	zw, err := gzip.NewWriterLevel(tmp, gzip.BestCompression)
	if err != nil {
		_ = src.Close()
		_ = tmp.Close()
		return err
	}
	_, copyErr := io.Copy(zw, src)
	_ = src.Close() // before anything is renamed or removed: Windows cannot do that to an open file
	if err := errors.Join(copyErr, zw.Close(), tmp.Sync(), tmp.Close()); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), info.Mode().Perm()); err != nil {
		return err
	}
	if err := os.Chtimes(tmp.Name(), info.ModTime(), info.ModTime()); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), plain+GzSuffix); err != nil {
		return err
	}
	return os.Remove(plain)
}

// Remove deletes the tape with the given ID, plain and compressed.
func Remove(dir, id string) error {
	var errs []error
	for _, name := range []string{FileName(id), FileName(id) + GzSuffix} {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
