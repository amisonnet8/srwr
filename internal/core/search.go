package core

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/amisonnet8/srwr/internal/ignore"
	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/tape"
	"github.com/amisonnet8/srwr/internal/token"
	"github.com/amisonnet8/srwr/internal/vcs"
)

// maxSearch is how many matching lines a search returns (and puts on the tape as looks).
const maxSearch = 20

// SearchInput is the input of look with search: the lines of a file that hold a text.
type SearchInput struct {
	File    string
	Search  string
	Why     string
	Include []string // only the files that match one of these patterns (.gitignore syntax); none: all
	Exclude []string // not the files that match one of these
	Offset  int      // the matches to skip before the first one returned
}

// SearchMatch is a line that holds the text: a token for that one line, the line, and the lines around it.
type SearchMatch struct {
	Selection string
	StartLine int
	EndLine   int
	Lines     []string
	Above     []string
	Below     []string
	File      string // set when a directory was searched
	Cut       bool   // a long line was cut to the part around the text
}

// FileCount is how many lines of a file hold the text.
type FileCount struct {
	File  string
	Count int
}

// SearchResult is what a search returns: how many lines hold the text, and the first maxSearch of them.
type SearchResult struct {
	Count   int
	Matches []SearchMatch
	Note    string      // when a directory was searched and had too many files to look in all of them
	ByFile  []FileCount // when a directory was searched and not all matches were returned: the files with the most, up to maxByFile
}

const (
	maxByFile  = 20
	maxLineLen = 200 // a line longer than this (in characters) is cut in the result
	cutAround  = 80  // the characters kept on each side of the text in a cut line
)

// filter is the include and exclude of a search.
type filter struct {
	include []*ignore.Matcher
	exclude *ignore.Matcher
}

func newFilter(in SearchInput) filter {
	f := filter{exclude: ignore.Compile(in.Exclude)}
	for _, p := range in.Include {
		if m := ignore.Compile([]string{p}); !m.Empty() {
			f.include = append(f.include, m)
		}
	}
	return f
}

// keeps tells whether a file is searched.
func (f filter) keeps(rel string) bool {
	if !f.exclude.Empty() && f.exclude.Match(rel) {
		return false
	}
	if len(f.include) == 0 {
		return true
	}
	for _, m := range f.include {
		if m.Match(rel) {
			return true
		}
	}
	return false
}

// cutLine shortens a line longer than maxLineLen to the part around the first place of text (or to its start, when text is not in it).
func cutLine(line, text string) (string, bool) {
	r := []rune(line)
	if len(r) <= maxLineLen {
		return line, false
	}
	i := strings.Index(line, text)
	if i < 0 {
		return string(r[:maxLineLen]) + "…", true // a line around the match: its start
	}
	at := len([]rune(line[:i]))
	start := max(at-cutAround, 0)
	end := min(at+len([]rune(text))+cutAround, len(r))
	out := string(r[start:end])
	if start > 0 {
		out = "…" + out
	}
	if end < len(r) {
		out += "…"
	}
	return out, true
}

// cutMatch shortens the lines of a match that are long. The token and the tape keep the whole line.
func cutMatch(m *SearchMatch, text string) {
	cut := func(lines []string) []string {
		out := make([]string, len(lines))
		for i, l := range lines {
			var c bool
			out[i], c = cutLine(l, text)
			m.Cut = m.Cut || c
		}
		return out
	}
	m.Lines, m.Above, m.Below = cut(m.Lines), cut(m.Above), cut(m.Below)
}

// maxSearchSize is the biggest file a search of a directory reads.
const maxSearchSize = 1 << 20

// Search finds the lines of a file that hold a text, and looks at each of them: one look on the tape for every match returned,
// with the same why. No match writes nothing. A failure is written to the tape (without the text searched for).
func (c *Core) Search(in SearchInput) (*SearchResult, *Error) {
	res, cerr := c.doSearch(in)
	if cerr != nil {
		c.recordFailure(failedCall{tool: toolLook, file: in.File, why: &in.Why, err: cerr})
	}
	return res, cerr
}

func (c *Core) doSearch(in SearchInput) (*SearchResult, *Error) {
	if err := checkWhy(in.Why); err != nil {
		return nil, err
	}
	switch {
	case in.Search == "":
		return nil, newError(CodeInvalidInput, "search must not be empty")
	case strings.ContainsAny(in.Search, "\r\n"):
		return nil, newError(CodeInvalidInput, "search is a text inside one line: it must not contain a line break")
	case in.Offset < 0:
		return nil, newError(CodeInvalidInput, "offset must not be negative")
	}
	dir, rel, cerr := c.searchTarget(in.File)
	if cerr != nil {
		return nil, cerr
	}
	var res *SearchResult
	cerr = c.run(func(tx *session.Tx) error {
		var err error
		if dir != "" {
			res, err = c.searchDir(tx, dir, in)
		} else {
			res, err = c.searchIn(tx, rel, in)
		}
		return err
	})
	if cerr != nil {
		return nil, cerr
	}
	return res, nil
}

func (c *Core) searchIn(tx *session.Tx, rel string, in SearchInput) (*SearchResult, error) {
	t, cerr := c.readTarget(rel)
	if cerr != nil {
		return nil, cerr
	}
	if err := c.observeTarget(tx, rel, "look", t); err != nil {
		return nil, err
	}
	res := &SearchResult{Matches: []SearchMatch{}}
	if !newFilter(in).keeps(rel) {
		return res, nil
	}
	for i, line := range tape.Lines(t.text) {
		if !strings.Contains(line, in.Search) {
			continue
		}
		res.Count++
		if res.Count <= in.Offset || len(res.Matches) >= maxSearch {
			continue
		}
		m, err := c.appendSearchLook(tx, rel, t.text, i+1, line, in.Why)
		if err != nil {
			return nil, err
		}
		cutMatch(&m, in.Search)
		res.Matches = append(res.Matches, m)
	}
	return res, nil
}

// searchTarget tells whether file is a directory to search below (dir, "." for the workspace) or a file (rel).
func (c *Core) searchTarget(file string) (dir, rel string, cerr *Error) {
	trimmed := strings.TrimSuffix(filepath.ToSlash(file), "/")
	if trimmed == "." {
		return ".", "", nil
	}
	rel, cerr = cleanPath(trimmed)
	if cerr != nil {
		return "", "", cerr
	}
	if info, err := os.Lstat(filepath.Join(c.WS.Root(), filepath.FromSlash(rel))); err == nil && info.IsDir() {
		return rel, "", nil
	}
	return "", rel, nil
}

// searchDir is the search of every file below dir that git lists (not the ones it ignores). A file that is not recorded, not text or
// bigger than maxSearchSize is left out without a word. Only the files with a match are put on the tape, each match as a look.
func (c *Core) searchDir(tx *session.Tx, dir string, in SearchInput) (*SearchResult, error) {
	files, more := vcs.ListFiles(c.WS.Root(), dir)
	res := &SearchResult{Matches: []SearchMatch{}}
	if more > 0 {
		res.Note = fmt.Sprintf("%d files were not searched: %s holds more than %d files. Search a smaller directory", more, dir, vcs.MaxListed)
	}
	keep := newFilter(in)
	counts := map[string]int{}
	for _, rel := range files {
		if !keep.keeps(rel) {
			continue
		}
		t, cerr := c.readTarget(rel)
		if cerr != nil || !t.exists || len(t.text) > maxSearchSize || !strings.Contains(t.text, in.Search) {
			continue
		}
		observed := false
		for i, line := range tape.Lines(t.text) {
			if !strings.Contains(line, in.Search) {
				continue
			}
			res.Count++
			counts[rel]++
			if res.Count <= in.Offset || len(res.Matches) >= maxSearch {
				continue
			}
			if !observed {
				if err := c.observeTarget(tx, rel, "look", t); err != nil {
					return nil, err
				}
				observed = true
			}
			m, err := c.appendSearchLook(tx, rel, t.text, i+1, line, in.Why)
			if err != nil {
				return nil, err
			}
			m.File = rel
			cutMatch(&m, in.Search)
			res.Matches = append(res.Matches, m)
		}
	}
	if res.Count > in.Offset+len(res.Matches) {
		for f, n := range counts {
			res.ByFile = append(res.ByFile, FileCount{File: f, Count: n})
		}
		slices.SortFunc(res.ByFile, func(a, b FileCount) int {
			if a.Count != b.Count {
				return b.Count - a.Count
			}
			return strings.Compare(a.File, b.File)
		})
		if len(res.ByFile) > maxByFile {
			res.ByFile = res.ByFile[:maxByFile]
		}
	}
	return res, nil
}

// appendSearchLook puts the look of the matching line n on the tape.
func (c *Core) appendSearchLook(tx *session.Tx, rel, text string, n int, line, why string) (SearchMatch, error) {
	sel := token.Encode(token.Token{
		Seq:       uint64(tx.NextSeq()), //nolint:gosec // NextSeq is at least 1
		StartLine: uint64(n),            //nolint:gosec // n is at least 1
		EndLine:   uint64(n),            //nolint:gosec // n is at least 1
		FileHash:  token.Hash4(rel),
		TextHash:  token.Hash4(line),
	}, tx.TapeID(), tx.Key())
	err := tx.Append(tape.Event{
		Type: tape.TypeLook, Seq: tx.NextSeq(), File: rel, StartLine: n, EndLine: n,
		Why: &why, Selection: &sel, Source: tape.SourceMCP,
	})
	if err != nil {
		return SearchMatch{}, err
	}
	return SearchMatch{
		Selection: sel, StartLine: n, EndLine: n, Lines: []string{line},
		Above: rangeLines(text, n-contextLines, n-1), Below: rangeLines(text, n+1, n+contextLines),
	}, nil
}
