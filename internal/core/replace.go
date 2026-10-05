package core

import (
	"fmt"
	"strings"

	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/tape"
	"github.com/amisonnet8/srwr/internal/token"
)

// ReplaceInput is the input of sub: replace every place that holds Old in Files with New, when there are exactly Count of them.
type ReplaceInput struct {
	Files []string
	Old   string
	New   string
	Count int
	Why   string
}

// ReplaceFile is what sub did to one file: how many places, and the range and token of the lines it changed (the first place to the last).
type ReplaceFile struct {
	File      string
	Hits      int
	StartLine int
	EndLine   int
	Selection string
	Lines     []string
}

// ReplaceResult is what sub returns: the number of places and, for each file that had any, what changed.
type ReplaceResult struct {
	Count int
	Files []ReplaceFile
}

// Replace replaces a text in several files. It changes nothing unless the number of places is Count. A failure is also written to
// the tape (without the texts).
func (c *Core) Replace(in ReplaceInput) (*ReplaceResult, *Error) {
	res, cerr := c.doReplace(in)
	if cerr != nil {
		c.recordFailure(failedCall{tool: toolReplace, files: in.Files, why: &in.Why, err: cerr})
	}
	return res, cerr
}

func (c *Core) doReplace(in ReplaceInput) (*ReplaceResult, *Error) {
	if err := checkWhy(in.Why); err != nil {
		return nil, err
	}
	switch {
	case len(in.Files) == 0:
		return nil, newError(CodeInvalidInput, "files is empty. Give the files to change")
	case in.Old == "":
		return nil, newError(CodeInvalidInput, "old is empty. Give the text to look for")
	case in.Count < 1:
		return nil, newError(CodeInvalidInput, "count must be 1 or more: the number of places you expect, in all the files")
	case strings.ContainsRune(in.Old, '\r') || strings.ContainsRune(in.New, '\r'):
		return nil, newError(CodeInvalidInput, "old and new must not contain CR (line breaks are LF only)")
	}
	rels := make([]string, len(in.Files))
	seen := map[string]bool{}
	for i, f := range in.Files {
		rel, cerr := cleanPath(f)
		if cerr != nil {
			return nil, cerr
		}
		if seen[rel] {
			return nil, newError(CodeInvalidInput, "%s is given more than once", rel)
		}
		seen[rel] = true
		rels[i] = rel
	}
	var res *ReplaceResult
	cerr := c.run(func(tx *session.Tx) error {
		var err error
		res, err = c.replaceIn(tx, rels, in)
		return err
	})
	if cerr != nil {
		return nil, cerr
	}
	return res, nil
}

// replacePlan is the change to one file, worked out before anything is written.
type replacePlan struct {
	rel      string
	t        target
	hits     int
	a, b     int // the lines of the text now, from the first place to the last (whole lines)
	oldText  string
	newLines []string
	newText  string
}

func (c *Core) replaceIn(tx *session.Tx, rels []string, in ReplaceInput) (*ReplaceResult, error) {
	plans := make([]replacePlan, len(rels))
	total := 0
	for i, rel := range rels {
		t, cerr := c.readTarget(rel)
		if cerr != nil {
			return nil, cerr
		}
		if err := c.observeTarget(tx, rel, "replace", t); err != nil {
			return nil, err
		}
		plans[i] = replacePlan{rel: rel, t: t, hits: strings.Count(t.text, in.Old)}
		total += plans[i].hits
	}
	if total != in.Count {
		per := make([]string, len(plans))
		actual := make(map[string]int, len(plans))
		for i, p := range plans {
			per[i] = fmt.Sprintf("%s: %d", p.rel, p.hits)
			actual[p.rel] = p.hits
		}
		return nil, &Error{
			Code:    CodeCountMismatch,
			Message: fmt.Sprintf("expected %d places of the text, found %d (%s). Nothing was changed", in.Count, total, strings.Join(per, ", ")),
			Actual:  actual,
		}
	}
	for i := range plans {
		if plans[i].hits == 0 {
			continue
		}
		if cerr := plans[i].plan(in); cerr != nil {
			return nil, cerr
		}
	}

	res := &ReplaceResult{Count: total}
	for _, p := range plans {
		if p.hits == 0 {
			continue
		}
		// The file first, then the tape (see editIn).
		if err := writeFile(p.t.real, p.newText); err != nil {
			return nil, err
		}
		newEnd := p.a + len(p.newLines) - 1
		sel := token.Encode(token.Token{
			Seq:       uint64(tx.NextSeq()), //nolint:gosec // NextSeq is at least 1
			StartLine: uint64(p.a),          //nolint:gosec // a is at least 1
			EndLine:   uint64(newEnd),       //nolint:gosec // newEnd is at least a-1
			FileHash:  token.Hash4(p.rel),
			TextHash:  token.Hash4(tape.RangeText(p.newText, p.a, newEnd)),
		}, tx.TapeID(), tx.Key())
		err := tx.Append(tape.Event{
			Type: tape.TypeReplace, Seq: tx.NextSeq(), File: p.rel,
			StartLine: p.a, EndLine: p.b, OldText: p.oldText, NewText: strings.Join(p.newLines, "\n"),
			NewStartLine: p.a, NewEndLine: newEnd, Selection: &sel, Why: &in.Why,
			FileShaBefore: tape.Sha(p.t.text), FileShaAfter: tape.Sha(p.newText),
			Source: tape.SourceMCP, Hits: p.hits,
		})
		if err != nil {
			return nil, err
		}
		res.Files = append(res.Files, ReplaceFile{
			File: p.rel, Hits: p.hits, StartLine: p.a, EndLine: newEnd, Selection: sel,
			Lines: rangeLines(p.newText, p.a, newEnd),
		})
	}
	return res, nil
}

// plan works out the lines that change: from the line of the first place to the line of the last, with every place in between
// replaced. It fails when the change cannot be told in lines (for example a line break added at the end of the file).
func (p *replacePlan) plan(in ReplaceInput) *Error {
	text := p.t.text
	first := strings.Index(text, in.Old)
	last := first
	for from := first + len(in.Old); ; {
		i := strings.Index(text[from:], in.Old)
		if i < 0 {
			break
		}
		last = from + i
		from = last + len(in.Old)
	}
	p.a = 1 + strings.Count(text[:first], "\n")
	p.b = 1 + strings.Count(text[:last+len(in.Old)-1], "\n")
	region := text[lineStart(text, p.a):lineEnd(text, p.b)]
	p.newLines = tape.Lines(strings.ReplaceAll(region, in.Old, in.New))
	p.oldText = tape.RangeText(text, p.a, p.b)
	p.newText = tape.SpliceLines(text, p.a, p.b, p.newLines)
	if p.newText != strings.ReplaceAll(text, in.Old, in.New) {
		return newError(CodeInvalidInput, "the change to %s cannot be written as lines (it adds or removes the final line break of the file). Use look and edit for it", p.rel)
	}
	return nil
}
