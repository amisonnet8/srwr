package core

import (
	"fmt"
	"strings"

	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/tape"
)

// Limits of look with looks: how many files, and how many lines in all.
const (
	maxLooks     = 10
	maxLooksSize = 2000
)

// LooksItem is one of the looks of a call with looks: a file, and the lines of it when HasLines (else the whole file).
type LooksItem struct {
	File      string
	StartLine int
	EndLine   int
	HasLines  bool
}

// LooksInput is the input of look with looks: several files read in one call, with one why.
type LooksInput struct {
	Items []LooksItem
	Why   string
}

// LooksResult is what looks returns: the result of each look, in the order given. Each has the File it is of.
type LooksResult struct {
	Items []LookItemResult
}

// LookItemResult is the result of one look of a call with looks.
type LookItemResult struct {
	File string
	LookResult
}

// Looks reads several files in one call: every item is checked before anything is put on the tape, so a call with a bad item
// records the failure and nothing else. Each item is then a look on the tape, with the same why. A failure is written to the tape
// (about the item that failed).
func (c *Core) Looks(in LooksInput) (*LooksResult, *Error) {
	res, bad, cerr := c.doLooks(in)
	if cerr != nil {
		f := failedCall{tool: toolLook, file: bad.File, why: &in.Why, err: cerr}
		if bad.HasLines {
			f.startLine, f.endLine = &bad.StartLine, &bad.EndLine
		}
		c.recordFailure(f)
	}
	return res, cerr
}

// inLook says in the message of an error which look of the call it is about.
func inLook(i int, e *Error) *Error {
	e.Message = fmt.Sprintf("looks[%d]: %s. Nothing was looked at: fix that item and send all the items again", i, strings.TrimSuffix(e.Message, "."))
	return e
}

func (c *Core) doLooks(in LooksInput) (res *LooksResult, bad LooksItem, cerr *Error) {
	if len(in.Items) == 0 || len(in.Items) > maxLooks {
		return nil, LooksItem{}, newError(CodeInvalidInput, "looks must hold 1 to %d items (it holds %d). Nothing was looked at", maxLooks, len(in.Items))
	}
	if err := checkWhy(in.Why); err != nil {
		return nil, LooksItem{}, err
	}
	rels := make([]string, len(in.Items))
	for i, item := range in.Items {
		rel, err := cleanPath(item.File)
		if err != nil {
			return nil, item, inLook(i, err)
		}
		rels[i] = rel
	}
	cerr = c.run(func(tx *session.Tx) error {
		var err error
		res, bad, err = c.looksIn(tx, in, rels)
		return err
	})
	if cerr != nil {
		return nil, bad, cerr
	}
	return res, LooksItem{}, nil
}

func (c *Core) looksIn(tx *session.Tx, in LooksInput, rels []string) (*LooksResult, LooksItem, error) {
	// First every range is found, from the files as they are; nothing is written until all of them are right.
	inputs := make([]LookInput, len(in.Items))
	total := 0
	for i, item := range in.Items {
		t, cerr := c.readTarget(rels[i])
		if cerr != nil {
			return nil, item, inLook(i, cerr)
		}
		if !t.exists {
			return nil, item, inLook(i, newError(CodeFileNotFound, "%s not found", rels[i]))
		}
		li := LookInput{File: rels[i], Why: in.Why, StartLine: item.StartLine, EndLine: item.EndLine}
		if !item.HasLines {
			li.StartLine, li.EndLine = 1, len(tape.Lines(t.text))
		}
		start, end, _, cerr := lookRange(rels[i], t.text, li)
		if cerr != nil {
			return nil, item, inLook(i, cerr)
		}
		inputs[i] = li
		if total += end - start + 1; total > maxLooksSize {
			return nil, item, inLook(i, newError(CodeInvalidInput, "the looks hold more than %d lines in all: read fewer files, or give startLine and endLine", maxLooksSize))
		}
	}
	res := &LooksResult{Items: make([]LookItemResult, len(in.Items))}
	for i, li := range inputs {
		r, err := c.lookIn(tx, rels[i], li)
		if err != nil {
			return nil, in.Items[i], err
		}
		res.Items[i] = LookItemResult{File: rels[i], LookResult: *r}
	}
	return res, LooksItem{}, nil
}
