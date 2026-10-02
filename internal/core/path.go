package core

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// cleanPath turns a path the client gave into a slash-separated path relative to the workspace.
func cleanPath(in string) (string, *Error) {
	if strings.TrimSpace(in) == "" || strings.ContainsRune(in, 0) {
		return "", newError(CodeInvalidInput, "file が空、または不正です")
	}
	p := filepath.ToSlash(in)
	if strings.HasPrefix(p, "/") || (len(p) >= 2 && p[1] == ':') {
		return "", newError(CodeInvalidRange, "%s は作業場の外のパス（作業場からの相対パスで指定してください）", in)
	}
	p = path.Clean(p)
	if p == "." || p == ".." || strings.HasPrefix(p, "../") {
		return "", newError(CodeInvalidRange, "%s は作業場の外のパス", in)
	}
	return p, nil
}

// target is a file of the workspace as it is now.
type target struct {
	real   string // the path to write to, with symbolic links followed
	text   string
	exists bool
}

// readTarget finds the file rel and reads it. A file that does not exist is not an error here:
// the caller records the deletion first.
func (c *Core) readTarget(rel string) (target, *Error) {
	full := filepath.Join(c.WS.Root(), filepath.FromSlash(rel))
	real, err := filepath.EvalSymlinks(full)
	if errors.Is(err, fs.ErrNotExist) {
		return target{}, nil
	}
	if err != nil {
		return target{}, internal(err)
	}
	realRoot, err := filepath.EvalSymlinks(c.WS.Root())
	if err != nil {
		return target{}, internal(err)
	}
	if within, err := filepath.Rel(realRoot, real); err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return target{}, newError(CodeInvalidRange, "%s は作業場の外を指している", rel)
	}
	info, err := os.Stat(real)
	if err != nil {
		return target{}, internal(err)
	}
	if !info.Mode().IsRegular() {
		return target{}, newError(CodeFileNotFound, "%s は通常のファイルではない", rel)
	}
	b, err := os.ReadFile(real)
	if err != nil {
		return target{}, internal(err)
	}
	if bytes.IndexByte(b, 0) >= 0 || !utf8.Valid(b) || bytes.IndexByte(b, '\r') >= 0 {
		return target{}, newError(CodeUnsupportedFile, "%s は CRLF などLF以外の改行、またはバイナリを含むため扱えない", rel)
	}
	return target{real: real, text: string(b), exists: true}, nil
}

// writeFile replaces the file at real with text: it writes a temporary file next to it and renames
// it over, so that a reader never sees half a file. The permissions are kept.
func writeFile(real, text string) error {
	info, err := os.Stat(real)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(real), ".srwr-*.tmp")
	if err != nil {
		return err
	}
	fail := func(err error) error {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if _, err := tmp.WriteString(text); err != nil {
		return fail(err)
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), real); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}

func internal(err error) *Error {
	return &Error{Code: CodeInternalError, Message: err.Error()}
}
