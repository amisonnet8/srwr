// Package core is what look, edit, replace and new mean: checking a range, correcting line numbers,
// matching content, noticing changes made outside srwr, and writing the file and the tape.
// It knows nothing of MCP; srwr mcp and srwr hook both come in through here.
package core

import (
	"errors"
	"fmt"
	"strings"

	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/tape"
	"github.com/amisonnet8/srwr/internal/token"
)

// Core works on one workspace.
type Core struct {
	WS *session.Workspace
}

// LookInput is the input of look.
type LookInput struct {
	File      string
	StartLine int
	EndLine   int
	Why       string

	// Expect is the content the range must hold (lines joined with "\n"). With Locate it is also how the range is found: StartLine
	// and EndLine are then not used. Its content never goes on the tape.
	Expect *string
	Locate bool
}

// LookResult is what look returns: a token for the range and the lines in it.
type LookResult struct {
	Selection string
	StartLine int
	EndLine   int
	Lines     []string

	// Note and LineCount are given when an endLine past the end of the file was cut to the end.
	Note      string
	LineCount int
}

// EditInput is the input of edit: a selection token, or a file with where to edit it. NewText and Why are always given.
type EditInput struct {
	Selection string
	NewText   string
	Why       string

	// By file (Selection is empty): the range is StartLine to EndLine when HasLines, and Expect, which never goes on the tape, is
	// what that range holds. Without line numbers, Expect says where the range is. An empty range (an insertion) needs no Expect.
	File      string
	StartLine int
	EndLine   int
	HasLines  bool
	Expect    *string

	// Old and New (both given, or neither) change a part of the text: the one place where Old is (in the lines StartLine to EndLine,
	// if HasLines) becomes New, and the lines it touches are the range. They are used with File only, not with Selection, Expect,
	// Insert or NewText. Neither goes on the tape.
	Old *string
	New *string
	// With Old, one of the line numbers is enough: OpenStart (no startLine) means from line 1, OpenEnd (no endLine) to the last line.
	// HasLines is then true, and the number not given is 0.
	OpenStart bool
	OpenEnd   bool

	// Insert, "after" or "before", keeps the range chosen (by the token, or by File and Expect) and puts NewText after or before it.
	Insert string
}

// contextLines is how many lines before and after the new range edit returns.
const contextLines = 2

// EditResult is what edit returns: a token for the new range, where it is, what it holds now, and the lines around it.
type EditResult struct {
	Selection string
	StartLine int
	EndLine   int
	Lines     []string
	Above     []string
	Below     []string

	// Hint, when there is one, tells the client of edits: it is given when the edit follows an edit of the same file with nothing
	// between them on the tape. It is not on the tape.
	Hint string
}

// EditsHint is what a single edit is told when it follows another edit of the same file.
const EditsHint = "Edits to one file in a row: edit with edits makes them in one call (one why, all or none; the ranges are found as the file is now, so no order is needed)."

// followsEdit tells whether the last event of the tape is an edit of the file made through srwr mcp.
func followsEdit(st *tape.State, rel string) bool {
	n := len(st.Edits)
	if n == 0 {
		return false
	}
	last := st.Edits[n-1]
	return last.Seq == st.LastSeq && last.Type == tape.TypeEdit && last.Source == tape.SourceMCP && last.File == rel
}

// run runs fn with the workspace lock. fn returns a *Error for a failure the client is told about;
// anything else is an internal error.
func (c *Core) run(fn func(tx *session.Tx) error) *Error {
	var cerr *Error
	err := c.WS.Do(func(tx *session.Tx) error {
		err := fn(tx)
		if errors.As(err, &cerr) {
			return nil // the lock is released as usual; the client gets the error
		}
		return err
	})
	if cerr != nil {
		return cerr
	}
	if err != nil {
		return internal(err)
	}
	return nil
}

// Look declares the range a client is looking at and returns a token to edit it with. A failure is also written to the tape.
func (c *Core) Look(in LookInput) (*LookResult, *Error) {
	res, cerr := c.doLook(in)
	if cerr != nil {
		f := failedCall{tool: toolLook, file: in.File, why: &in.Why, err: cerr}
		if !in.Locate {
			f.startLine, f.endLine = &in.StartLine, &in.EndLine
		}
		c.recordFailure(f)
	}
	return res, cerr
}

func (c *Core) doLook(in LookInput) (*LookResult, *Error) {
	if err := checkWhy(in.Why); err != nil {
		return nil, err
	}
	rel, cerr := cleanPath(in.File)
	if cerr != nil {
		return nil, cerr
	}
	var res *LookResult
	cerr = c.run(func(tx *session.Tx) error {
		var err error
		res, err = c.lookIn(tx, rel, in)
		return err
	})
	if cerr != nil {
		return nil, cerr
	}
	return res, nil
}

func (c *Core) lookIn(tx *session.Tx, rel string, in LookInput) (*LookResult, error) {
	t, cerr := c.readTarget(rel)
	if cerr != nil {
		return nil, cerr
	}
	if err := c.observeTarget(tx, rel, "look", t); err != nil {
		return nil, err
	}

	start, end, note, cerr := lookRange(rel, t.text, in)
	if cerr != nil {
		return nil, cerr
	}

	sel := token.Encode(token.Token{
		Seq:       uint64(tx.NextSeq()), //nolint:gosec // NextSeq is at least 1
		StartLine: uint64(start),        //nolint:gosec // checked in chooseRange: at least 1
		EndLine:   uint64(end),          //nolint:gosec // checked in chooseRange: at least start-1, so 0 or more
		FileHash:  token.Hash4(rel),
		TextHash:  token.Hash4(tape.RangeText(t.text, start, end)),
	}, tx.TapeID(), tx.Key())

	err := tx.Append(tape.Event{
		Type: tape.TypeLook, Seq: tx.NextSeq(), File: rel, StartLine: start, EndLine: end,
		Why: &in.Why, Selection: &sel, Source: tape.SourceMCP,
	})
	if err != nil {
		return nil, err
	}
	res := &LookResult{Selection: sel, StartLine: start, EndLine: end, Lines: rangeLines(t.text, start, end), Note: note}
	if note != "" {
		res.LineCount = len(tape.Lines(t.text))
	}
	return res, nil
}

// lookRange finds the lines a look covers. A look only reads, so an endLine past the end of the file is cut to the end (startLine
// must still be in the file); note says so.
func lookRange(rel, text string, in LookInput) (start, end int, note string, cerr *Error) {
	if n := len(tape.Lines(text)); !in.Locate && in.StartLine >= 1 && in.StartLine <= n+1 && in.EndLine > n {
		note = fmt.Sprintf("endLine %d is past the end of %s (%d lines): the range ends at line %d", in.EndLine, rel, n, n)
		in.EndLine = n
	}
	start, end, cerr = chooseRange(rel, text, in)
	return start, end, note, cerr
}

// Edit puts new text in the range a token stands for. A failure is also written to the tape (without the new text).
func (c *Core) Edit(in EditInput) (*EditResult, *Error) {
	res, cerr := c.doEdit(in)
	if cerr != nil {
		f := editFailure(in, in.Why, cerr)
		c.recordFailure(f)
	}
	return res, cerr
}

func (c *Core) doEdit(in EditInput) (*EditResult, *Error) {
	byFile, rel, cerr := checkEdit(in)
	if cerr != nil {
		return nil, cerr
	}
	var res *EditResult
	cerr = c.run(func(tx *session.Tx) error {
		p, err := c.resolve(tx, byFile, rel, in, map[string]*target{})
		if err != nil {
			return err
		}
		follows := followsEdit(tx.State(), p.rel)
		res, err = c.writeEdit(tx, p.rel, p.t, p.a, p.b, p.textOf(in), in.Why, p.from)
		if err == nil && follows {
			res.Hint = EditsHint
		}
		return err
	})
	if cerr != nil {
		return nil, cerr
	}
	return res, nil
}

// checkEdit checks the input of one edit before the workspace is touched. byFile is true when the range is pointed at by file and
// expect, and rel is then the cleaned path.
func checkEdit(in EditInput) (byFile bool, rel string, cerr *Error) {
	byFile = strings.TrimSpace(in.Selection) == ""
	if cerr := checkOld(in, byFile); cerr != nil {
		return false, "", cerr
	}
	switch {
	case byFile && in.File == "":
		return false, "", newError(CodeInvalidInput, "give selection (from look), or file with expect (and startLine and endLine, if you know them)")
	case !byFile && (in.File != "" || in.HasLines || in.Expect != nil || in.Old != nil):
		return false, "", newError(CodeInvalidInput, "give selection, or file with expect; not both")
	}
	if err := checkWhy(in.Why); err != nil {
		return false, "", err
	}
	if strings.ContainsRune(in.NewText, '\r') {
		return false, "", newError(CodeInvalidInput, "newText must not contain CR (line breaks are LF only)")
	}
	switch {
	case in.Insert != "" && in.Insert != InsertAfter && in.Insert != InsertBefore && in.Insert != InsertStart && in.Insert != InsertEnd:
		return false, "", newError(CodeInvalidInput, "insert must be \"after\", \"before\", \"start\" or \"end\"")
	case (in.Insert == InsertStart || in.Insert == InsertEnd) && (!byFile || in.HasLines || in.Expect != nil):
		return false, "", newError(CodeInvalidInput, "insert %q puts newText at the start (or end) of the file: give file and newText only, no selection, expect or line numbers", in.Insert)
	case in.Insert != "" && in.HasLines && in.EndLine == in.StartLine-1:
		return false, "", newError(CodeInvalidInput, "with insert, point at lines to put the text next to (an empty range has none): give expect, and startLine and endLine of those lines")
	}
	if byFile {
		if rel, cerr = cleanPath(in.File); cerr != nil {
			return false, "", cerr
		}
		if in.Expect != nil && strings.ContainsRune(*in.Expect, '\r') {
			return false, "", newError(CodeInvalidInput, "expect must not contain CR (line breaks are LF only)")
		}
	}
	return byFile, rel, nil
}

// checkOld checks the input of an edit with old and new.
func checkOld(in EditInput, byFile bool) *Error {
	if in.Old == nil && in.New == nil {
		return nil
	}
	switch {
	case in.Old == nil || in.New == nil:
		return newError(CodeInvalidInput, "give old and new together")
	case !byFile:
		return newError(CodeInvalidInput, "old and new go with file, not with selection")
	case in.Expect != nil || in.Insert != "" || in.NewText != "":
		return newError(CodeInvalidInput, "old and new are not given with expect, newText or insert: they change a part of the text by themselves")
	case *in.Old == "":
		return newError(CodeInvalidInput, "old is empty. Give the text to change")
	case strings.ContainsRune(*in.Old, '\r') || strings.ContainsRune(*in.New, '\r'):
		return newError(CodeInvalidInput, "old and new must not contain CR (line breaks are LF only)")
	case *in.Old == *in.New:
		return newError(CodeInvalidInput, "old and new are the same: nothing would change")
	case in.HasLines && !in.OpenStart && !in.OpenEnd && in.EndLine < in.StartLine:
		return newError(CodeInvalidInput, "with old, startLine and endLine are the lines to look in: give at least one line")
	}
	return nil
}

// editPlan is where one edit puts its text: the lines a..b of the file rel (an empty range for an insertion), found but not yet written.
type editPlan struct {
	rel  string
	t    target
	a, b int
	from *string // the token the range came from, if any
	put  *string // with old and new: the text that takes the place of the lines a..b (otherwise the NewText of the edit)
}

// textOf is the text an edit of the plan puts in.
func (p editPlan) textOf(in EditInput) string {
	if p.put != nil {
		return *p.put
	}
	return putIn(in)
}

// loadTarget reads the file rel and records what Observe finds, once for each file of a call: seen holds the files already read.
func (c *Core) loadTarget(tx *session.Tx, rel, detectedBy string, seen map[string]*target) (target, error) {
	if t, ok := seen[rel]; ok {
		return *t, nil
	}
	t, cerr := c.readTarget(rel)
	if cerr != nil {
		return target{}, cerr
	}
	if err := c.observeTarget(tx, rel, detectedBy, t); err != nil {
		return target{}, err
	}
	seen[rel] = &t
	return t, nil
}

// resolve finds the range of an edit that checkEdit accepted, by its token or by file and expect, and applies insert to it.
func (c *Core) resolve(tx *session.Tx, byFile bool, rel string, in EditInput, seen map[string]*target) (editPlan, error) {
	if byFile {
		return c.resolveFile(tx, rel, in, seen)
	}
	return c.resolveToken(tx, in, seen)
}

func (c *Core) resolveToken(tx *session.Tx, in EditInput, seen map[string]*target) (editPlan, error) {
	tok, err := token.Decode(in.Selection, tx.TapeID(), tx.Key())
	if err != nil {
		return editPlan{}, newError(CodeInvalidSelection, "the selection token is not valid: it was altered, or issued in another session. Call look again")
	}
	rel, ok := findFile(tx.State(), tok.FileHash)
	if !ok {
		return editPlan{}, newError(CodeInvalidSelection, "the file of the selection token is not on the tape. Call look again")
	}
	t, err := c.loadTarget(tx, rel, "edit", seen)
	if err != nil {
		return editPlan{}, err
	}

	a, b, ok := Correct(tx.State().Edits, rel, int(tok.Seq), int(tok.StartLine), int(tok.EndLine)) //nolint:gosec // line numbers and seq are far below the int range
	if !ok {
		return editPlan{}, &Error{
			Code:    CodeSelectionStale,
			Message: "an edit overlapped the range after the look. Call look again",
			Actual:  rangeLines(t.text, a, b),
		}
	}
	n := len(tape.Lines(t.text))
	oldText := tape.RangeText(t.text, a, b)
	if a < 1 || b > n || b < a-1 || token.Hash4(oldText) != tok.TextHash {
		return editPlan{}, &Error{
			Code:    CodeSelectionMismatch,
			Message: "even with the line numbers corrected, the range differs from what look returned (it may have been changed outside srwr). Check the content and call look again",
			Actual:  rangeLines(t.text, a, b),
		}
	}

	a, b = insertAt(a, b, in.Insert)
	return editPlan{rel: rel, t: t, a: a, b: b, from: &in.Selection}, nil
}

// resolveFile finds the range chosen by locateEdit in a file given by its path.
func (c *Core) resolveFile(tx *session.Tx, rel string, in EditInput, seen map[string]*target) (editPlan, error) {
	t, err := c.loadTarget(tx, rel, "edit", seen)
	if err != nil {
		return editPlan{}, err
	}
	if in.Old != nil {
		a, b, put, cerr := locateOld(tx.State(), rel, t.text, in)
		if cerr != nil {
			return editPlan{}, cerr
		}
		return editPlan{rel: rel, t: t, a: a, b: b, put: &put}, nil
	}
	if in.Insert == InsertStart || in.Insert == InsertEnd {
		n := len(tape.Lines(t.text))
		if in.Insert == InsertStart {
			return editPlan{rel: rel, t: t, a: 1, b: 0}, nil
		}
		return editPlan{rel: rel, t: t, a: n + 1, b: n}, nil
	}
	a, b, cerr := locateEdit(tx.State(), rel, t.text, in)
	if cerr != nil {
		return editPlan{}, cerr
	}
	a, b = insertAt(a, b, in.Insert)
	return editPlan{rel: rel, t: t, a: a, b: b}, nil
}

// putIn is the text an edit puts in. With insert, an empty NewText is one empty line (without insert it deletes).
func putIn(in EditInput) string {
	if in.Insert != "" && in.NewText == "" {
		return "\n"
	}
	return in.NewText
}

// The values of EditInput.Insert.
const (
	InsertAfter  = "after"
	InsertBefore = "before"
	InsertStart  = "start" // at the top of the file
	InsertEnd    = "end"   // at the bottom of the file
)

// insertAt turns the range a..b to keep into the empty range where the new text goes: just after b, or just before a.
func insertAt(a, b int, insert string) (int, int) {
	switch insert {
	case InsertAfter:
		return b + 1, b
	case InsertBefore:
		return a, a - 1
	}
	return a, b
}

// writeEdit puts newText in lines a..b of the file, and records it. from is the token the range came from, if any.
func (c *Core) writeEdit(tx *session.Tx, rel string, t target, a, b int, newTextIn, why string, from *string) (*EditResult, error) {
	oldText := tape.RangeText(t.text, a, b)
	newLines := tape.Lines(newTextIn)
	newText := tape.SpliceLines(t.text, a, b, newLines)
	newEnd := a + len(newLines) - 1

	// The file first, then the tape: if we die in between, the next call finds the file
	// different from the tape and records it as external.
	if err := writeFile(t.real, newText); err != nil {
		return nil, err
	}
	sel := token.Encode(token.Token{
		Seq:       uint64(tx.NextSeq()), //nolint:gosec // NextSeq is at least 1
		StartLine: uint64(a),            //nolint:gosec // a is at least 1, checked by the caller
		EndLine:   uint64(newEnd),       //nolint:gosec // newEnd is at least a-1
		FileHash:  token.Hash4(rel),
		TextHash:  token.Hash4(tape.RangeText(newText, a, newEnd)),
	}, tx.TapeID(), tx.Key())
	err := tx.Append(tape.Event{
		Type: tape.TypeEdit, Seq: tx.NextSeq(), File: rel, From: from,
		StartLine: a, EndLine: b, OldText: oldText, NewText: strings.Join(newLines, "\n"),
		NewStartLine: a, NewEndLine: newEnd, Selection: &sel, Why: &why,
		FileShaBefore: tape.Sha(t.text), FileShaAfter: tape.Sha(newText), Source: tape.SourceMCP,
	})
	if err != nil {
		return nil, err
	}
	res := &EditResult{
		Selection: sel, StartLine: a, EndLine: newEnd,
		Lines: rangeLines(newText, a, newEnd),
		Above: rangeLines(newText, a-contextLines, a-1),
		Below: rangeLines(newText, newEnd+1, newEnd+contextLines),
	}
	if b < a {
		res.Hint = touchHint(newText, a, newEnd)
	}
	return res, nil
}

// touchHint is what an insertion of lines a..end of text is told when its first (last) line sits against a line of the same
// indentation with no empty line between: two blocks (functions, paragraphs) put together are usually meant to be apart. The
// insertion is at least 2 lines; a single line is not a block.
func touchHint(text string, a, end int) string {
	if end-a < 1 {
		return ""
	}
	lines := tape.Lines(text)
	indent := func(l string) int { return len(l) - len(strings.TrimLeft(l, " \t")) }
	touches := func(in, next int) bool { // line numbers; next is the line outside the insertion
		if next < 1 || next > len(lines) {
			return false
		}
		x, y := lines[in-1], lines[next-1]
		return strings.TrimSpace(x) != "" && strings.TrimSpace(y) != "" && indent(x) == indent(y)
	}
	var msgs []string
	if touches(a, a-1) {
		msgs = append(msgs, "The inserted lines touch the line above with no empty line between. If they are a separate block, start newText with an empty line.")
	}
	if touches(end, end+1) {
		msgs = append(msgs, "The inserted lines touch the line below with no empty line between. If they are a separate block, end newText with an empty line.")
	}
	return strings.Join(msgs, " ")
}

// observeTarget records what Observe finds. For a file that is gone it then reports file_not_found.
func (c *Core) observeTarget(tx *session.Tx, rel, detectedBy string, t target) error {
	if !t.exists {
		if err := Observe(tx, rel, detectedBy, nil); err != nil {
			return err
		}
		return newError(CodeFileNotFound, "%s not found", rel)
	}
	return Observe(tx, rel, detectedBy, &t.text)
}

// findFile finds the file a token is about: the one of the tape whose path has the token's hash.
func findFile(st *tape.State, fileHash [4]byte) (string, bool) {
	found := ""
	for name := range st.Files {
		if token.Hash4(name) == fileHash && (found == "" || name < found) {
			found = name
		}
	}
	return found, found != ""
}

func checkWhy(why string) *Error {
	if strings.TrimSpace(why) == "" {
		return newError(CodeInvalidInput, "why is required (blank is not allowed). Say in one sentence why you are doing this")
	}
	return nil
}
