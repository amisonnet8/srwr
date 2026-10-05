// Package core is what look, edit, replace and new mean: checking a range, correcting line numbers,
// matching content, noticing changes made outside srwr, and writing the file and the tape.
// It knows nothing of MCP; srwr mcp and srwr hook both come in through here.
package core

import (
	"errors"
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
	Before    []string
	After     []string
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

	start, end, cerr := chooseRange(rel, t.text, in)
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
	return &LookResult{Selection: sel, StartLine: start, EndLine: end, Lines: rangeLines(t.text, start, end)}, nil
}

// Edit puts new text in the range a token stands for. A failure is also written to the tape (without the new text).
func (c *Core) Edit(in EditInput) (*EditResult, *Error) {
	res, cerr := c.doEdit(in)
	if cerr != nil {
		f := failedCall{tool: toolEdit, why: &in.Why, err: cerr}
		if in.Selection != "" || in.File == "" {
			f.selection = &in.Selection
		} else {
			f.file = in.File
			if in.HasLines {
				f.startLine, f.endLine = &in.StartLine, &in.EndLine
			}
		}
		c.recordFailure(f)
	}
	return res, cerr
}

func (c *Core) doEdit(in EditInput) (*EditResult, *Error) {
	byFile := strings.TrimSpace(in.Selection) == ""
	switch {
	case byFile && in.File == "":
		return nil, newError(CodeInvalidInput, "give selection (from look), or file with expect (and startLine and endLine, if you know them)")
	case !byFile && (in.File != "" || in.HasLines || in.Expect != nil):
		return nil, newError(CodeInvalidInput, "give selection, or file with expect; not both")
	}
	if err := checkWhy(in.Why); err != nil {
		return nil, err
	}
	if strings.ContainsRune(in.NewText, '\r') {
		return nil, newError(CodeInvalidInput, "newText must not contain CR (line breaks are LF only)")
	}
	switch {
	case in.Insert != "" && in.Insert != InsertAfter && in.Insert != InsertBefore:
		return nil, newError(CodeInvalidInput, "insert must be \"after\" or \"before\"")
	case in.Insert != "" && in.NewText == "":
		return nil, newError(CodeInvalidInput, "insert needs newText: the lines to put in (for an empty line, newText is a line break)")
	case in.Insert != "" && in.HasLines && in.EndLine == in.StartLine-1:
		return nil, newError(CodeInvalidInput, "with insert, point at lines to put the text next to (an empty range has none): give expect, and startLine and endLine of those lines")
	}
	var rel string
	if byFile {
		var cerr *Error
		if rel, cerr = cleanPath(in.File); cerr != nil {
			return nil, cerr
		}
		if in.Expect != nil && strings.ContainsRune(*in.Expect, '\r') {
			return nil, newError(CodeInvalidInput, "expect must not contain CR (line breaks are LF only)")
		}
	}
	var res *EditResult
	cerr := c.run(func(tx *session.Tx) error {
		var err error
		if byFile {
			res, err = c.editFileIn(tx, rel, in)
		} else {
			res, err = c.editIn(tx, in)
		}
		return err
	})
	if cerr != nil {
		return nil, cerr
	}
	return res, nil
}

func (c *Core) editIn(tx *session.Tx, in EditInput) (*EditResult, error) {
	tok, err := token.Decode(in.Selection, tx.TapeID(), tx.Key())
	if err != nil {
		return nil, newError(CodeInvalidSelection, "the selection token is not valid: it was altered, or issued in another session. Call look again")
	}
	rel, ok := findFile(tx.State(), tok.FileHash)
	if !ok {
		return nil, newError(CodeInvalidSelection, "the file of the selection token is not on the tape. Call look again")
	}
	t, cerr := c.readTarget(rel)
	if cerr != nil {
		return nil, cerr
	}
	if err := c.observeTarget(tx, rel, "edit", t); err != nil {
		return nil, err
	}

	a, b, ok := Correct(tx.State().Edits, rel, int(tok.Seq), int(tok.StartLine), int(tok.EndLine)) //nolint:gosec // line numbers and seq are far below the int range
	if !ok {
		return nil, &Error{
			Code:    CodeSelectionStale,
			Message: "an edit overlapped the range after the look. Call look again",
			Actual:  rangeLines(t.text, a, b),
		}
	}
	n := len(tape.Lines(t.text))
	oldText := tape.RangeText(t.text, a, b)
	if a < 1 || b > n || b < a-1 || token.Hash4(oldText) != tok.TextHash {
		return nil, &Error{
			Code:    CodeSelectionMismatch,
			Message: "even with the line numbers corrected, the range differs from what look returned (it may have been changed outside srwr). Check the content and call look again",
			Actual:  rangeLines(t.text, a, b),
		}
	}

	a, b = insertAt(a, b, in.Insert)
	return c.writeEdit(tx, rel, t, a, b, in.NewText, in.Why, &in.Selection)
}

// editFileIn edits the range chosen by locateEdit in a file given by its path.
func (c *Core) editFileIn(tx *session.Tx, rel string, in EditInput) (*EditResult, error) {
	t, cerr := c.readTarget(rel)
	if cerr != nil {
		return nil, cerr
	}
	if err := c.observeTarget(tx, rel, "edit", t); err != nil {
		return nil, err
	}
	a, b, cerr := locateEdit(tx.State(), rel, t.text, in)
	if cerr != nil {
		return nil, cerr
	}
	a, b = insertAt(a, b, in.Insert)
	return c.writeEdit(tx, rel, t, a, b, in.NewText, in.Why, nil)
}

// The values of EditInput.Insert.
const (
	InsertAfter  = "after"
	InsertBefore = "before"
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
	return &EditResult{
		Selection: sel, StartLine: a, EndLine: newEnd,
		Lines:  rangeLines(newText, a, newEnd),
		Before: rangeLines(newText, a-contextLines, a-1),
		After:  rangeLines(newText, newEnd+1, newEnd+contextLines),
	}, nil
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
