package core

import (
	"fmt"
	"strings"

	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/tape"
)

// maxHitsShown is how many places a file's result lists; the rest are counted in More.
const maxHitsShown = 20

// ReplaceInput is the input of replace: replace every place that holds Old in Files with New, when there are exactly Count of them.
// Count is 2 or more: one place is for edit (use_edit).
type ReplaceInput struct {
	Files []string
	Old   string
	New   string
	Count int
	Why   string
}

// ReplaceHit is one place that replace changed, as the file is now: its lines, and the line before and the line after. Places on the
// same line are one.
type ReplaceHit struct {
	StartLine int
	EndLine   int
	Lines     []string
	Above     []string
	Below     []string
}

// ReplaceFile is what replace did to one file: how many places, and the places (at most maxHitsShown; More is how many were left out).
type ReplaceFile struct {
	File  string
	Count int
	Hits  []ReplaceHit
	More  int
}

// ReplaceResult is what replace returns: the number of places and, for each file that had any, what changed.
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
		return nil, newError(CodeInvalidInput, "count must be 2 or more: the number of places you expect, in all the files (for one place, use edit)")
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
	if in.Count == 1 && total == 1 {
		return nil, useEdit(plans, in)
	}
	if total != in.Count {
		per := make([]string, len(plans))
		actual := make(map[string]int, len(plans))
		for i, p := range plans {
			per[i] = fmt.Sprintf("%s: %d", p.rel, p.hits)
			actual[p.rel] = p.hits
		}
		e := &Error{
			Code:    CodeCountMismatch,
			Message: fmt.Sprintf("expected %d places of the text, found %d (%s). Nothing was changed", in.Count, total, strings.Join(per, ", ")),
			Actual:  actual,
		}
		if total < in.Count {
			nearReplace(e, plans, in.Old)
		}
		return nil, e
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
		err := tx.Append(tape.Event{
			Type: tape.TypeReplace, Seq: tx.NextSeq(), File: p.rel,
			StartLine: p.a, EndLine: p.b, OldText: p.oldText, NewText: strings.Join(p.newLines, "\n"),
			NewStartLine: p.a, NewEndLine: newEnd, Why: &in.Why,
			FileShaBefore: tape.Sha(p.t.text), FileShaAfter: tape.Sha(p.newText),
			Source: tape.SourceMCP, Hits: p.hits,
		})
		if err != nil {
			return nil, err
		}
		hits := hitsOf(p.t.text, in.Old, in.New)
		more := max(len(hits)-maxHitsShown, 0)
		res.Files = append(res.Files, ReplaceFile{File: p.rel, Count: p.hits, Hits: hits[:len(hits)-more], More: more})
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

// hitsOf lists the places where old is replaced by repl in text, as lines of the text after the change. Places that share a line
// are one.
func hitsOf(text, old, repl string) []ReplaceHit {
	var offsets []int // where each replacement starts in the new text
	var b strings.Builder
	for from := 0; ; {
		i := strings.Index(text[from:], old)
		if i < 0 {
			b.WriteString(text[from:])
			break
		}
		b.WriteString(text[from : from+i])
		offsets = append(offsets, b.Len())
		b.WriteString(repl)
		from += i + len(old)
	}
	after := b.String()
	var hits []ReplaceHit
	for _, off := range offsets {
		end := off
		if len(repl) > 0 {
			end = off + len(repl) - 1
		}
		start, last := 1+strings.Count(after[:off], "\n"), 1+strings.Count(after[:end], "\n")
		if n := len(hits); n > 0 && start <= hits[n-1].EndLine {
			hits[n-1].EndLine = max(hits[n-1].EndLine, last)
			continue
		}
		hits = append(hits, ReplaceHit{StartLine: start, EndLine: last})
	}
	for i := range hits {
		h := &hits[i]
		h.Lines = rangeLines(after, h.StartLine, h.EndLine)
		h.Above = rangeLines(after, h.StartLine-1, h.StartLine-1)
		h.Below = rangeLines(after, h.EndLine+1, h.EndLine+1)
	}
	return hits
}

// useEditHit and useEditCall are the parts of the actual of use_edit: where the one place is now, and the edit call that changes it
// (the client adds why).
type useEditHit struct {
	File      string   `json:"file"`
	StartLine int      `json:"startLine"`
	EndLine   int      `json:"endLine"`
	Lines     []string `json:"lines"`
}

type useEditCall struct {
	File string `json:"file"`
	Old  string `json:"old"`
	New  string `json:"new"`
}

// useEdit is the answer to a replace of one place: it changes nothing, and says where the place is and what to call edit with. The
// message has no content of the file (it is written to the tape).
func useEdit(plans []replacePlan, in ReplaceInput) *Error {
	for i := range plans {
		p := &plans[i]
		if p.hits != 1 {
			continue
		}
		cerr := p.plan(in) // sets the lines even when it then fails: the change cannot be told in lines
		where := fmt.Sprintf("%s line %d", p.rel, p.a)
		if p.b > p.a {
			where = fmt.Sprintf("%s lines %d to %d", p.rel, p.a, p.b)
		}
		actual := map[string]any{"hits": []useEditHit{{File: p.rel, StartLine: p.a, EndLine: p.b, Lines: rangeLines(p.t.text, p.a, p.b)}}}
		msg := fmt.Sprintf("replace is for 2 or more places, and the text is in one place only (%s). Use edit for it", where)
		if cerr == nil && !overlapping(p.t.text, in.Old) {
			actual["edit"] = useEditCall{File: p.rel, Old: in.Old, New: in.New}
			msg += ": actual.edit is the call to make (add why)"
		}
		return &Error{Code: CodeUseEdit, Message: msg, Actual: actual}
	}
	return newError(CodeInternalError, "no place found")
}

// nearReplace adds the places of old that differ only in spaces and tabs, in every file. The message names the files and lines.
func nearReplace(e *Error, plans []replacePlan, old string) {
	var all []NearMatch
	var where []string
	for _, p := range plans {
		for _, m := range nearText(p.t.text, old) {
			if len(all) == maxNear {
				break
			}
			m.File = p.rel
			all = append(all, m)
			where = append(where, fmt.Sprintf("%s line %d", p.rel, m.StartLine))
		}
	}
	if len(all) == 0 {
		return
	}
	e.Message += fmt.Sprintf(". Differing from old only in spaces or tabs: %s (see nearMatches)", strings.Join(where, ", "))
	e.NearMatches = all
}

// overlapping tells whether old is in text in two places that share characters ("aa" in "aaa"): edit counts those as two.
func overlapping(text, old string) bool {
	n := 0
	for from := 0; from < len(text); {
		i := strings.Index(text[from:], old)
		if i < 0 {
			break
		}
		n++
		from += i + 1
	}
	return n > 1
}
