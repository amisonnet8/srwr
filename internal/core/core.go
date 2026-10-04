// Package core is what select and replace mean: checking a range, correcting line numbers,
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

// SelectInput is the input of select.
type SelectInput struct {
	File      string
	StartLine int
	EndLine   int
	Why       string

	// Expect is the content the range must hold (lines joined with "\n"). With Locate it is also how the range is found: StartLine
	// and EndLine are then not used. Its content never goes on the tape.
	Expect *string
	Locate bool
}

// SelectResult is what select returns: a token for the range and the lines in it.
type SelectResult struct {
	Selection string
	StartLine int
	EndLine   int
	Lines     []string
}

// ReplaceInput is the input of replace.
type ReplaceInput struct {
	Selection string
	NewText   string
	Why       string
}

// contextLines is how many lines before and after the new range replace returns.
const contextLines = 2

// ReplaceResult is what replace returns: a token for the new range, where it is, what it holds now, and the lines around it.
type ReplaceResult struct {
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

// Select declares the range a client is looking at and returns a token to edit it with. A failure is also written to the tape.
func (c *Core) Select(in SelectInput) (*SelectResult, *Error) {
	res, cerr := c.doSelect(in)
	if cerr != nil {
		f := failedCall{tool: toolSelect, file: in.File, why: &in.Why, err: cerr}
		if !in.Locate {
			f.startLine, f.endLine = &in.StartLine, &in.EndLine
		}
		c.recordFailure(f)
	}
	return res, cerr
}

func (c *Core) doSelect(in SelectInput) (*SelectResult, *Error) {
	if err := checkWhy(in.Why); err != nil {
		return nil, err
	}
	rel, cerr := cleanPath(in.File)
	if cerr != nil {
		return nil, cerr
	}
	var res *SelectResult
	cerr = c.run(func(tx *session.Tx) error {
		var err error
		res, err = c.selectIn(tx, rel, in)
		return err
	})
	if cerr != nil {
		return nil, cerr
	}
	return res, nil
}

func (c *Core) selectIn(tx *session.Tx, rel string, in SelectInput) (*SelectResult, error) {
	t, cerr := c.readTarget(rel)
	if cerr != nil {
		return nil, cerr
	}
	if err := c.observeTarget(tx, rel, "select", t); err != nil {
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
		Type: tape.TypeSelect, Seq: tx.NextSeq(), File: rel, StartLine: start, EndLine: end,
		Why: &in.Why, Selection: &sel, Source: tape.SourceMCP,
	})
	if err != nil {
		return nil, err
	}
	return &SelectResult{Selection: sel, StartLine: start, EndLine: end, Lines: rangeLines(t.text, start, end)}, nil
}

// Replace puts new text in the range a token stands for. A failure is also written to the tape (without the new text).
func (c *Core) Replace(in ReplaceInput) (*ReplaceResult, *Error) {
	res, cerr := c.doReplace(in)
	if cerr != nil {
		c.recordFailure(failedCall{tool: toolReplace, selection: &in.Selection, why: &in.Why, err: cerr})
	}
	return res, cerr
}

func (c *Core) doReplace(in ReplaceInput) (*ReplaceResult, *Error) {
	if strings.TrimSpace(in.Selection) == "" {
		return nil, newError(CodeInvalidInput, "selection is empty")
	}
	if err := checkWhy(in.Why); err != nil {
		return nil, err
	}
	if strings.ContainsRune(in.NewText, '\r') {
		return nil, newError(CodeInvalidInput, "newText must not contain CR (line breaks are LF only)")
	}
	var res *ReplaceResult
	cerr := c.run(func(tx *session.Tx) error {
		var err error
		res, err = c.replaceIn(tx, in)
		return err
	})
	if cerr != nil {
		return nil, cerr
	}
	return res, nil
}

func (c *Core) replaceIn(tx *session.Tx, in ReplaceInput) (*ReplaceResult, error) {
	tok, err := token.Decode(in.Selection, tx.TapeID(), tx.Key())
	if err != nil {
		return nil, newError(CodeInvalidSelection, "the selection token is not valid: it was altered, or issued in another session. Call select again")
	}
	rel, ok := findFile(tx.State(), tok.FileHash)
	if !ok {
		return nil, newError(CodeInvalidSelection, "the file of the selection token is not on the tape. Call select again")
	}
	t, cerr := c.readTarget(rel)
	if cerr != nil {
		return nil, cerr
	}
	if err := c.observeTarget(tx, rel, "replace", t); err != nil {
		return nil, err
	}

	a, b, ok := Correct(tx.State().Replaces, rel, int(tok.Seq), int(tok.StartLine), int(tok.EndLine)) //nolint:gosec // line numbers and seq are far below the int range
	if !ok {
		return nil, &Error{
			Code:    CodeSelectionStale,
			Message: "an edit overlapped the range after the select. Call select again",
			Actual:  rangeLines(t.text, a, b),
		}
	}
	n := len(tape.Lines(t.text))
	oldText := tape.RangeText(t.text, a, b)
	if a < 1 || b > n || b < a-1 || token.Hash4(oldText) != tok.TextHash {
		return nil, &Error{
			Code:    CodeSelectionMismatch,
			Message: "even with the line numbers corrected, the range differs from what select returned (it may have been changed outside srwr). Check the content and call select again",
			Actual:  rangeLines(t.text, a, b),
		}
	}

	newLines := tape.Lines(in.NewText)
	newText := tape.SpliceLines(t.text, a, b, newLines)
	newEnd := a + len(newLines) - 1

	// The file first, then the tape: if we die in between, the next call finds the file
	// different from the tape and records it as external.
	if err := writeFile(t.real, newText); err != nil {
		return nil, err
	}
	sel := token.Encode(token.Token{
		Seq:       uint64(tx.NextSeq()), //nolint:gosec // NextSeq is at least 1
		StartLine: uint64(a),
		EndLine:   uint64(newEnd),
		FileHash:  tok.FileHash,
		TextHash:  token.Hash4(tape.RangeText(newText, a, newEnd)),
	}, tx.TapeID(), tx.Key())
	err = tx.Append(tape.Event{
		Type: tape.TypeReplace, Seq: tx.NextSeq(), File: rel, From: &in.Selection,
		StartLine: a, EndLine: b, OldText: oldText, NewText: strings.Join(newLines, "\n"),
		NewStartLine: a, NewEndLine: newEnd, Selection: &sel, Why: &in.Why,
		FileShaBefore: tape.Sha(t.text), FileShaAfter: tape.Sha(newText), Source: tape.SourceMCP,
	})
	if err != nil {
		return nil, err
	}
	return &ReplaceResult{
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
