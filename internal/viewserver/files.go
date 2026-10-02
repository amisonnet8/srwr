package viewserver

import (
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

func itoa(n int) string { return strconv.Itoa(n) }

// readCurrent reads a file of the workspace as it is now, for the last diff. rel comes from a tape,
// and a tape may have been written by anyone, so it is not trusted: a path that is not plainly
// inside the workspace, or that leads out of it through a symbolic link, is read as a file that
// does not exist. The text is made valid UTF-8, because it is sent as JSON.
func (s *Server) readCurrent(rel string) (text string, exists bool) {
	if rel == "" || strings.ContainsRune(rel, 0) || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) ||
		filepath.IsAbs(rel) || (len(rel) >= 2 && rel[1] == ':') {
		return "", false
	}
	clean := path.Clean(filepath.ToSlash(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	real, err := filepath.EvalSymlinks(filepath.Join(s.Root, filepath.FromSlash(clean)))
	if err != nil {
		return "", false
	}
	realRoot, err := filepath.EvalSymlinks(s.Root)
	if err != nil {
		return "", false
	}
	within, err := filepath.Rel(realRoot, real)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", false
	}
	info, err := os.Stat(real)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	b, err := os.ReadFile(real)
	if err != nil {
		return "", false
	}
	return strings.ToValidUTF8(string(b), "�"), true
}
