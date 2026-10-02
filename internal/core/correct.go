package core

import "github.com/amisonnet8/srwr/internal/tape"

// Correct moves the range a..b of a token to where its lines are now, following the replaces that
// came after the token was issued (docs/design/token.md). replaces is the list of every replace in
// the tape, in order. It returns the corrected range; ok is false when an edit overlaps the range, and
// then the range is how far it was corrected before that edit.
//
// An edit s..e is above the range when e < a (the range moves by how many lines the edit added),
// below it when s > b (nothing changes), and overlaps it otherwise. An empty range has b = a-1,
// and so does an insertion (e = s-1).
func Correct(replaces []tape.Event, file string, afterSeq, a, b int) (int, int, bool) {
	for _, r := range replaces {
		if r.File != file || r.Seq <= afterSeq {
			continue
		}
		s, e := r.StartLine, r.EndLine
		switch {
		case e < a:
			delta := len(tape.NewLines(r)) - (e - s + 1)
			a += delta
			b += delta
		case s > b:
		default:
			return a, b, false
		}
	}
	return a, b, true
}

// rangeLines returns the lines a..b of text, cut to what the text has. It is never nil, so that it
// is written as [] when the range is empty.
func rangeLines(text string, a, b int) []string {
	lines := tape.Lines(text)
	a = max(a, 1)
	b = min(b, len(lines))
	if a > b {
		return []string{}
	}
	return append([]string{}, lines[a-1:b]...)
}
