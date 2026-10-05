package core

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/tape"
	"github.com/amisonnet8/srwr/internal/token"
)

// maxEdits is how many edits one call may hold.
const maxEdits = 50

// EditsInput is the input of edit with edits: several edits made in one call, with one why. The Why of each item is not used.
type EditsInput struct {
	Edits []EditInput
	Why   string
}

// EditsResult is what edits returns: the result of each edit, in the order they were given.
type EditsResult struct {
	Edits []EditResult
}

// Edits makes several edits in one call, all or none. Every range is found as the files are before the call (so the line numbers
// and expect of an item do not depend on the other items), and they must not overlap. The tape gets one edit for each item, with the
// same why. A failure is also written to the tape, as one failure about the item that failed (without the new text).
func (c *Core) Edits(in EditsInput) (*EditsResult, *Error) {
	res, bad, cerr := c.doEdits(in)
	if cerr != nil {
		f := editFailure(bad, in.Why, cerr)
		c.recordFailure(f)
	}
	return res, cerr
}

// editFailure is what the tape knows of a failed edit: the item that failed (an empty one when the call as a whole did).
func editFailure(item EditInput, why string, cerr *Error) failedCall {
	f := failedCall{tool: toolEdit, why: &why, err: cerr}
	if item.Selection != "" || item.File == "" {
		f.selection = &item.Selection
	} else {
		f.file = item.File
		if item.HasLines {
			f.startLine, f.endLine = &item.StartLine, &item.EndLine
		}
	}
	return f
}

// itemPlan is an edit of a call with several: where it goes and what goes there.
type itemPlan struct {
	editPlan
	idx      int
	newLines []string
}

// step is an edit worked out against the file as the edits above it have changed it, ready to be written to the tape.
type step struct {
	idx                 int
	rel                 string
	from                *string
	a, b, newEnd        int
	oldText, newText    string
	shaBefore, shaAfter string
	rangeAfter          string // the new range, for the hash of its token
}

func (c *Core) doEdits(in EditsInput) (res *EditsResult, bad EditInput, cerr *Error) {
	if len(in.Edits) == 0 || len(in.Edits) > maxEdits {
		return nil, EditInput{}, newError(CodeInvalidInput, "edits must hold 1 to %d edits (it holds %d)", maxEdits, len(in.Edits))
	}
	if err := checkWhy(in.Why); err != nil {
		return nil, EditInput{}, err
	}
	items := make([]EditInput, len(in.Edits))
	byFile := make([]bool, len(items))
	rels := make([]string, len(items))
	for i, item := range in.Edits {
		item.Why = in.Why
		items[i] = item
		var err *Error
		if byFile[i], rels[i], err = checkEdit(item); err != nil {
			return nil, item, inItem(i, err)
		}
	}

	cerr = c.run(func(tx *session.Tx) error {
		var err error
		res, bad, err = c.editsIn(tx, items, byFile, rels, in.Why)
		return err
	})
	if cerr != nil {
		return nil, bad, cerr
	}
	return res, EditInput{}, nil
}

// inItem says in the message of an error which edit of the call it is about.
func inItem(i int, e *Error) *Error {
	e.Message = fmt.Sprintf("edits[%d]: %s", i, e.Message)
	return e
}

func (c *Core) editsIn(tx *session.Tx, items []EditInput, byFile []bool, rels []string, why string) (*EditsResult, EditInput, error) {
	// Every range is found before anything is written, from the state of the files and of the tape as the call found them.
	seen := map[string]*target{}
	plans := make([]itemPlan, len(items))
	var files []string // in the order they first appear
	for i, item := range items {
		p, err := c.resolve(tx, byFile[i], rels[i], item, seen)
		if err != nil {
			var cerr *Error
			if errors.As(err, &cerr) {
				return nil, item, inItem(i, cerr)
			}
			return nil, item, err
		}
		plans[i] = itemPlan{editPlan: p, idx: i, newLines: tape.Lines(putIn(item))}
		if !slices.Contains(files, p.rel) {
			files = append(files, p.rel)
		}
	}

	byRel := map[string][]itemPlan{}
	for _, p := range plans {
		byRel[p.rel] = append(byRel[p.rel], p)
	}
	for _, rel := range files {
		group := byRel[rel]
		slices.SortFunc(group, func(x, y itemPlan) int {
			if x.a != y.a {
				return x.a - y.a
			}
			return x.b - y.b
		})
		for k := 1; k < len(group); k++ {
			if overlaps(group[k-1], group[k]) {
				x, y := group[k-1].idx, group[k].idx
				return nil, EditInput{}, newError(CodeInvalidInput, "edits[%d] and edits[%d] overlap in %s: the ranges of one call must be apart (nothing was changed)",
					min(x, y), max(x, y), rel)
			}
		}
		byRel[rel] = group
	}

	// Work out each file from the top, and write all the files before the tape.
	var steps []step
	finals := map[string]string{}
	for _, rel := range files {
		text := seen[rel].text
		delta := 0
		for _, p := range byRel[rel] {
			a, b := p.a+delta, p.b+delta
			next := tape.SpliceLines(text, a, b, p.newLines)
			newEnd := a + len(p.newLines) - 1
			steps = append(steps, step{
				idx: p.idx, rel: rel, from: p.from, a: a, b: b, newEnd: newEnd,
				oldText: tape.RangeText(text, a, b), newText: strings.Join(p.newLines, "\n"),
				shaBefore: tape.Sha(text), shaAfter: tape.Sha(next), rangeAfter: tape.RangeText(next, a, newEnd),
			})
			delta += len(p.newLines) - (b - a + 1)
			text = next
		}
		finals[rel] = text
	}
	for _, rel := range files {
		if err := writeFile(seen[rel].real, finals[rel]); err != nil {
			return nil, EditInput{}, err
		}
	}

	res := &EditsResult{Edits: make([]EditResult, len(items))}
	for _, st := range steps {
		sel := token.Encode(token.Token{
			Seq:       uint64(tx.NextSeq()), //nolint:gosec // NextSeq is at least 1
			StartLine: uint64(st.a),         //nolint:gosec // a is at least 1
			EndLine:   uint64(st.newEnd),    //nolint:gosec // newEnd is at least a-1
			FileHash:  token.Hash4(st.rel),
			TextHash:  token.Hash4(st.rangeAfter),
		}, tx.TapeID(), tx.Key())
		err := tx.Append(tape.Event{
			Type: tape.TypeEdit, Seq: tx.NextSeq(), File: st.rel, From: st.from,
			StartLine: st.a, EndLine: st.b, OldText: st.oldText, NewText: st.newText,
			NewStartLine: st.a, NewEndLine: st.newEnd, Selection: &sel, Why: &why,
			FileShaBefore: st.shaBefore, FileShaAfter: st.shaAfter, Source: tape.SourceMCP,
		})
		if err != nil {
			return nil, EditInput{}, err
		}
		final := finals[st.rel]
		res.Edits[st.idx] = EditResult{
			Selection: sel, StartLine: st.a, EndLine: st.newEnd,
			Lines: rangeLines(final, st.a, st.newEnd),
			Above: rangeLines(final, st.a-contextLines, st.a-1),
			Below: rangeLines(final, st.newEnd+1, st.newEnd+contextLines),
		}
	}
	return res, EditInput{}, nil
}

// overlaps tells whether two ranges of one file, x before y in the order of their lines, cannot both be made. Lines shared, or an
// insertion inside the other range, or two insertions at one place (their order would not be said).
func overlaps(x, y itemPlan) bool {
	xEmpty, yEmpty := x.b < x.a, y.b < y.a
	switch {
	case xEmpty && yEmpty:
		return x.a == y.a
	case xEmpty:
		return false // x is before y's first line, or at it: the text goes in first
	case yEmpty:
		return y.a > x.a && y.a <= x.b
	}
	return y.a <= x.b
}
