package core

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/amisonnet8/srwr/internal/tape"
)

// How many places a message names.
const (
	maxFoundAt   = 5  // content_mismatch: where the same lines are
	maxAmbiguous = 10 // content_ambiguous: where they are
)

// chooseRange decides the range a look stands for: the line numbers as given, or the place where Expect is found. With Expect and
// line numbers, the range must hold the lines of Expect. A failure is a *Error.
func chooseRange(rel, text string, in LookInput) (start, end int, cerr *Error) {
	lines := tape.Lines(text)
	n := len(lines)

	var want []string
	if in.Expect != nil {
		if strings.ContainsRune(*in.Expect, '\r') {
			return 0, 0, newError(CodeInvalidInput, "expect must not contain CR (line breaks are LF only)")
		}
		want = tape.Lines(*in.Expect)
	}

	if in.Locate {
		if len(want) == 0 {
			return 0, 0, newError(CodeInvalidInput, "without startLine and endLine, expect must hold at least one line (to point at a place to insert, give line numbers)")
		}
		at := findLines(lines, want)
		switch len(at) {
		case 0:
			return 0, 0, newError(CodeContentNotFound, "the lines of expect are not in %s (%d lines). Check the content, or give startLine and endLine", rel, n)
		case 1:
			return at[0], at[0] + len(want) - 1, nil
		}
		return 0, 0, newError(CodeContentAmbiguous, "the lines of expect are in %s in %d places: %s. Give startLine and endLine, or more lines in expect",
			rel, len(at), places(at, maxAmbiguous))
	}

	start, end = in.StartLine, in.EndLine
	if start < 1 || start > n+1 || end < start-1 || end > n {
		return 0, 0, &Error{
			Code:    CodeInvalidRange,
			Message: fmt.Sprintf("%s has %d lines; startLine=%d endLine=%d is out of range", rel, n, start, end),
			Actual:  map[string]int{"lineCount": n},
		}
	}
	if in.Expect == nil || slices.Equal(rangeLines(text, start, end), want) {
		return start, end, nil
	}
	msg := fmt.Sprintf("lines %d to %d of %s are not the lines of expect.", start, end, rel)
	if at := findLines(lines, want); len(want) > 0 && len(at) > 0 {
		msg += " The same lines are at " + places(at, maxFoundAt) + ". Look again with those line numbers"
	} else {
		msg += " They are not in the file. Read the file again"
	}
	return 0, 0, &Error{Code: CodeContentMismatch, Message: msg, Actual: rangeLines(text, start, end)}
}

// findLines returns the first line of every place where the lines want are, one after the other, in lines. Places may overlap.
func findLines(lines, want []string) []int {
	var at []int
	for i := 0; i+len(want) <= len(lines); i++ {
		if slices.Equal(lines[i:i+len(want)], want) {
			at = append(at, i+1)
		}
	}
	return at
}

// places is "line 14" or "lines 14, 31", cut after limit places.
func places(at []int, limit int) string {
	more := len(at) > limit
	if more {
		at = at[:limit]
	}
	s := make([]string, len(at))
	for i, v := range at {
		s[i] = strconv.Itoa(v)
	}
	switch {
	case more:
		return "lines " + strings.Join(s, ", ") + " and more"
	case len(s) == 1:
		return "line " + s[0]
	}
	return "lines " + strings.Join(s, ", ")
}
