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
			return 0, 0, withNear(newError(CodeContentNotFound, "the lines of expect are not in %s (%d lines). Check the content, or give startLine and endLine", rel, n),
				"", nearLines(lines, want), "expect")
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
	e := &Error{Code: CodeContentMismatch, Message: msg, Actual: rangeLines(text, start, end)}
	if len(want) > 0 && len(findLines(lines, want)) == 0 {
		_ = withNear(e, "", nearLines(lines, want), "expect")
	}
	return 0, 0, e
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

// locateEdit decides the range an edit by file stands for. The line numbers as given are tried first, then the same numbers
// moved to where the file's lines are now, following the changes after the file's last look (the lines the client saw), then the
// one place where Expect is found. A range that holds the lines of Expect is the only thing taken for it. A failure is a *Error.
func locateEdit(st *tape.State, rel, text string, in EditInput) (start, end int, cerr *Error) {
	lines := tape.Lines(text)
	n := len(lines)

	var want []string
	if in.Expect != nil {
		want = tape.Lines(*in.Expect)
	}

	if in.HasLines && in.EndLine == in.StartLine-1 { // an insertion: there are no lines to check
		if len(want) > 0 {
			return 0, 0, newError(CodeInvalidInput, "an empty range (endLine = startLine - 1) inserts before line %d; expect must be empty or left out", in.StartLine)
		}
		if in.StartLine < 1 || in.StartLine > n+1 {
			return 0, 0, &Error{
				Code:    CodeInvalidRange,
				Message: fmt.Sprintf("%s has %d lines; startLine=%d endLine=%d is out of range", rel, n, in.StartLine, in.EndLine),
				Actual:  map[string]int{"lineCount": n},
			}
		}
		// Nothing says the place is still the one the client meant, unless the file has not moved since a look.
		if look, ok := st.LastLook[rel]; !ok || look < st.LastChange[rel] {
			return 0, 0, newError(CodeContentNotFound, "%s changed after the last look (or was not looked at), so line %d may not be the place you mean. Call look again, or edit the line before it with expect and put the new line in newText", rel, in.StartLine)
		}
		return in.StartLine, in.EndLine, nil
	}

	if len(want) == 0 {
		return 0, 0, newError(CodeInvalidInput, "expect must hold at least one line (to insert, give startLine and endLine = startLine - 1)")
	}

	var cands []int // first lines of the candidates, from the line numbers as given and from the corrected ones
	if in.HasLines {
		if in.StartLine < 1 || in.StartLine > n+1 || in.EndLine < in.StartLine-1 || in.EndLine > n {
			return 0, 0, &Error{
				Code:    CodeInvalidRange,
				Message: fmt.Sprintf("%s has %d lines; startLine=%d endLine=%d is out of range", rel, n, in.StartLine, in.EndLine),
				Actual:  map[string]int{"lineCount": n},
			}
		}
		if slices.Equal(rangeLines(text, in.StartLine, in.EndLine), want) {
			cands = append(cands, in.StartLine)
		}
		if a, b, ok := correctedFromLook(st, rel, in); ok && a != in.StartLine && a >= 1 && b <= n && slices.Equal(rangeLines(text, a, b), want) {
			cands = append(cands, a)
		}
	}
	if len(cands) == 0 {
		cands = findLines(lines, want)
	}
	switch len(cands) {
	case 1:
		return cands[0], cands[0] + len(want) - 1, nil
	case 0:
		e := newError(CodeContentNotFound, "the lines of expect are not in %s (%d lines). Check the content, or call look again", rel, n)
		_ = withNear(e, "", nearLines(lines, want), "expect")
		if in.HasLines && in.StartLine >= 1 {
			e.Actual = rangeLines(text, in.StartLine, in.EndLine)
		}
		return 0, 0, e
	}
	return 0, 0, newError(CodeContentAmbiguous, "the lines of expect are in %s in %d places: %s. Give startLine and endLine, or more lines in expect",
		rel, len(cands), places(cands, maxAmbiguous))
}

// correctedFromLook moves the line numbers of in to where they are now, from the last look of the file. ok is false when the file
// was not looked at, or a change overlaps the range.
func correctedFromLook(st *tape.State, rel string, in EditInput) (int, int, bool) {
	look, ok := st.LastLook[rel]
	if !ok {
		return in.StartLine, in.EndLine, false
	}
	return Correct(st.Edits, rel, look, in.StartLine, in.EndLine)
}
