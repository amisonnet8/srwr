package core

import (
	"strings"

	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/tape"
	"github.com/amisonnet8/srwr/internal/token"
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
}

// SearchResult is what a search returns: how many lines hold the text, and the first maxSearch of them.
type SearchResult struct {
	Count   int
	Matches []SearchMatch
}

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
	rel, cerr := cleanPath(in.File)
	if cerr != nil {
		return nil, cerr
	}
	var res *SearchResult
	cerr = c.run(func(tx *session.Tx) error {
		var err error
		res, err = c.searchIn(tx, rel, in)
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
		n := i + 1
		sel := token.Encode(token.Token{
			Seq:       uint64(tx.NextSeq()), //nolint:gosec // NextSeq is at least 1
			StartLine: uint64(n),
			EndLine:   uint64(n),
			FileHash:  token.Hash4(rel),
			TextHash:  token.Hash4(line),
		}, tx.TapeID(), tx.Key())
		err := tx.Append(tape.Event{
			Type: tape.TypeLook, Seq: tx.NextSeq(), File: rel, StartLine: n, EndLine: n,
			Why: &in.Why, Selection: &sel, Source: tape.SourceMCP,
		})
		if err != nil {
			return nil, err
		}
		res.Matches = append(res.Matches, SearchMatch{
			Selection: sel, StartLine: n, EndLine: n, Lines: []string{line},
			Above: rangeLines(t.text, n-contextLines, n-1), Below: rangeLines(t.text, n+1, n+contextLines),
		})
	}
	return res, nil
}
