package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/tape"
	"github.com/amisonnet8/srwr/internal/token"
	"github.com/amisonnet8/srwr/internal/vcs"
)

// maxSearch is how many matching lines a search returns (and puts on the tape as looks).
const maxSearch = 20

// SearchInput is the input of look with search: the lines of a file that hold a text.
type SearchInput struct {
	File   string
	Search string
	Why    string
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
}

// SearchResult is what a search returns: how many lines hold the text, and the first maxSearch of them.
type SearchResult struct {
	Count   int
	Matches []SearchMatch
	Note    string // when a directory was searched and had too many files to look in all of them
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
	for i, line := range tape.Lines(t.text) {
		if !strings.Contains(line, in.Search) {
			continue
		}
		res.Count++
		if len(res.Matches) >= maxSearch {
			continue
		}
		m, err := c.appendSearchLook(tx, rel, t.text, i+1, line, in.Why)
		if err != nil {
			return nil, err
		}
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
	for _, rel := range files {
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
			if len(res.Matches) >= maxSearch {
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
			res.Matches = append(res.Matches, m)
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
