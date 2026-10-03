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

	"github.com/amisonnet8/srwr/internal/ignore"
)

// cleanPath turns a path the client gave into a slash-separated path relative to the workspace.
func cleanPath(in string) (string, *Error) {
	if strings.TrimSpace(in) == "" || strings.ContainsRune(in, 0) {
		return "", newError(CodeInvalidInput, "file is empty or not valid")
	}
	p := filepath.ToSlash(in)
	if strings.HasPrefix(p, "/") || (len(p) >= 2 && p[1] == ':') {
		return "", newError(CodeInvalidRange, "%s is a path outside the workspace (give a path relative to the workspace)", in)
	}
	p = path.Clean(p)
	if p == "." || p == ".." || strings.HasPrefix(p, "../") {
		return "", newError(CodeInvalidRange, "%s is a path outside the workspace", in)
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
//
// This is the one place every entrance goes through (select, replace, hook, the look for external changes), so it is where
// the files that are never recorded are turned away, whether they exist or not and whether they are named directly or
// through a symbolic link.
func (c *Core) readTarget(rel string) (target, *Error) {
	// A .srwrignore that cannot be read leaves us unable to say what is secret: nothing is recorded then.
	m, err := ignore.Load(c.WS.Root())
	if err != nil {
		return target{}, internal(err)
	}
	if m.Match(rel) {
		return target{}, ignoredError(rel)
	}
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
		return target{}, newError(CodeInvalidRange, "%s points outside the workspace", rel)
	}
	if within, err := filepath.Rel(realRoot, real); err == nil && m.Match(filepath.ToSlash(within)) {
		return target{}, ignoredError(rel)
	}
	info, err := os.Stat(real)
	if err != nil {
		return target{}, internal(err)
	}
	if !info.Mode().IsRegular() {
		return target{}, newError(CodeFileNotFound, "%s is not a regular file", rel)
	}
	b, err := os.ReadFile(real)
	if err != nil {
		return target{}, internal(err)
	}
	if bytes.IndexByte(b, 0) >= 0 || !utf8.Valid(b) || bytes.IndexByte(b, '\r') >= 0 {
		return target{}, newError(CodeUnsupportedFile, "%s cannot be handled: it has line breaks other than LF (CRLF, for example) or is binary", rel)
	}
	return target{real: real, text: string(b), exists: true}, nil
}

func ignoredError(rel string) *Error {
	return newError(CodeIgnoredFile, "%s is a file that is not recorded, so srwr cannot handle it. Ask the user", rel)
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
