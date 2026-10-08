// Package ignore decides which files of a workspace are never put on a tape (docs/reference/cli.md, "Files that are not recorded").
// The built-in patterns cannot be undone; the .srwrignore of the workspace root adds to them in the syntax of .gitignore.
package ignore

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// FileName is the file of the workspace root that names more files to leave out.
const FileName = ".srwrignore"

// builtin are the files that are always left out, in .gitignore syntax.
var builtin = []string{".env", ".env.*", "*.pem", "*.key", "id_rsa*", "id_ed25519*", "*.p12", "*.pfx", ".srwr/"}

type rule struct {
	segs     []string // pattern split at "/", lower case
	anchored bool     // the pattern has a "/" in it: it is matched from the workspace root
	dirOnly  bool     // the pattern ended with "/"
	negate   bool
}

// Matcher says whether a file is left out.
type Matcher struct {
	builtin []rule
	user    []rule
}

// Load reads .srwrignore of the workspace root. A workspace without one has only the built-in patterns. A .srwrignore that
// cannot be read is an error: the caller must not record anything then.
func Load(root string) (*Matcher, error) {
	m := &Matcher{}
	for _, p := range builtin {
		if r, ok := parse(p); ok {
			m.builtin = append(m.builtin, r)
		}
	}
	f, err := os.Open(filepath.Join(root, FileName)) //nolint:gosec // the ignore file of the workspace
	if errors.Is(err, fs.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		if r, ok := parse(sc.Text()); ok {
			m.user = append(m.user, r)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return m, nil
}

// parse reads one line. Blank lines and comments are not rules.
func parse(line string) (rule, bool) {
	line = strings.TrimSuffix(line, "\r")
	// Trailing spaces are dropped unless escaped with a backslash.
	for strings.HasSuffix(line, " ") && !strings.HasSuffix(line, "\\ ") {
		line = line[:len(line)-1]
	}
	if line == "" || strings.HasPrefix(line, "#") {
		return rule{}, false
	}
	var r rule
	if strings.HasPrefix(line, "!") {
		r.negate = true
		line = line[1:]
	}
	if strings.HasSuffix(line, "/") {
		r.dirOnly = true
		line = strings.TrimRight(line, "/")
	}
	if strings.Contains(line, "/") {
		r.anchored = true
		line = strings.TrimLeft(line, "/")
	}
	if line == "" {
		return rule{}, false
	}
	for _, s := range strings.Split(strings.ToLower(line), "/") {
		r.segs = append(r.segs, strings.ReplaceAll(s, "[!", "[^"))
	}
	return r, true
}

// Match reports whether rel (a slash-separated path from the workspace root) is left out. A file in a directory that is left
// out is left out too, and cannot be brought back. Case does not matter: .ENV opens .env on macOS and Windows.
func (m *Matcher) Match(rel string) bool {
	rel = strings.ToLower(path.Clean(filepath.ToSlash(rel)))
	comps := strings.Split(rel, "/")
	for k := 1; k <= len(comps); k++ {
		prefix, isDir := comps[:k], k < len(comps)
		for _, r := range m.builtin {
			if r.matches(prefix, isDir) {
				return true
			}
		}
		out := false
		for _, r := range m.user {
			if r.matches(prefix, isDir) {
				out = !r.negate
			}
		}
		if out {
			return true
		}
	}
	return false
}

func (r rule) matches(comps []string, isDir bool) bool {
	if r.dirOnly && !isDir {
		return false
	}
	if !r.anchored {
		ok, err := path.Match(r.segs[0], comps[len(comps)-1])
		return err == nil && ok
	}
	return matchSegs(r.segs, comps)
}

// matchSegs matches the pattern segments against the path components. "**" is zero or more components, and at the end of a
// pattern one or more (everything inside).
func matchSegs(pat, comps []string) bool {
	if len(pat) == 0 {
		return len(comps) == 0
	}
	if pat[0] == "**" {
		if len(pat) == 1 {
			return len(comps) >= 1
		}
		for i := 0; i <= len(comps); i++ {
			if matchSegs(pat[1:], comps[i:]) {
				return true
			}
		}
		return false
	}
	if len(comps) == 0 {
		return false
	}
	ok, err := path.Match(pat[0], comps[0])
	return err == nil && ok && matchSegs(pat[1:], comps[1:])
}

// Compile makes a Matcher of the given patterns (.gitignore syntax, blank lines are skipped), with no built-in patterns. It is for
// choosing files (the include and exclude of a search), not for leaving them out of the tape.
func Compile(patterns []string) *Matcher {
	m := &Matcher{}
	for _, p := range patterns {
		if r, ok := parse(p); ok {
			m.user = append(m.user, r)
		}
	}
	return m
}

// Empty tells whether the matcher has no pattern at all.
func (m *Matcher) Empty() bool { return len(m.builtin) == 0 && len(m.user) == 0 }
