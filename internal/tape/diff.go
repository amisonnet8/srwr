package tape

import "strings"

// Hunk is one place where an external change touched a file: lines StartLine..EndLine of the text
// before became NewText (NewStartLine..NewEndLine of the text after). Same meaning as in a replace,
// without the old text (the text before is known from the tape). An insertion has EndLine = StartLine-1;
// a deletion has an empty NewText and NewEndLine = NewStartLine-1.
type Hunk struct {
	StartLine    int    `json:"startLine"`
	EndLine      int    `json:"endLine"`
	NewText      string `json:"newText"`
	NewStartLine int    `json:"newStartLine"`
	NewEndLine   int    `json:"newEndLine"`
}

// newLines returns the lines the hunk wrote. NewText cannot say by itself whether it ends in a blank
// line, so the count comes from NewStartLine and NewEndLine (the same as NewLines for a replace).
func (h Hunk) newLines() []string {
	n := h.NewEndLine - h.NewStartLine + 1
	if n <= 0 {
		return nil
	}
	if parts := strings.Split(h.NewText, "\n"); len(parts) == n {
		return parts
	}
	return Lines(h.NewText)
}

// maxEdits is how many lines may differ before Diff gives up (the whole text is written instead).
const maxEdits = 1000

// Diff returns the hunks that turn before into after, by lines. ok is false when the texts differ
// by more than maxEdits lines. The hunks do not tell whether the text ends in a line break, so
// a caller checks ApplyHunks(before, hunks) == after before it trusts them.
func Diff(before, after string) (hunks []Hunk, ok bool) {
	a, b := Lines(before), Lines(after)
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	a, b = a[p:len(a)-s], b[p:len(b)-s]
	delA, insB, ok := myers(a, b)
	if !ok {
		return nil, false
	}
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		if (i >= len(a) || !delA[i]) && (j >= len(b) || !insB[j]) {
			i++
			j++
			continue
		}
		si, sj := i, j
		for (i < len(a) && delA[i]) || (j < len(b) && insB[j]) {
			if i < len(a) && delA[i] {
				i++
			}
			if j < len(b) && insB[j] {
				j++
			}
		}
		hunks = append(hunks, Hunk{
			StartLine: p + si + 1, EndLine: p + i,
			NewText: strings.Join(b[sj:j], "\n"), NewStartLine: p + sj + 1, NewEndLine: p + j,
		})
	}
	return hunks, true
}

// ApplyHunks applies hunks (lines of the text before, from top to bottom) to text.
func ApplyHunks(text string, hunks []Hunk) string {
	for k := len(hunks) - 1; k >= 0; k-- { // from the bottom, so the line numbers above stay right
		h := hunks[k]
		text = SpliceLines(text, h.StartLine, h.EndLine, h.newLines())
	}
	return text
}

// myers finds a shortest edit script from a to b (Myers, O(ND)). delA[i] says a[i] is removed and
// insB[j] says b[j] is added; the lines left over are the same, in the same order.
func myers(a, b []string) (delA, insB []bool, ok bool) {
	n, m := len(a), len(b)
	delA, insB = make([]bool, n), make([]bool, m)
	if n == 0 && m == 0 {
		return delA, insB, true
	}
	limit := min(n+m, maxEdits)
	off := limit + 1
	v := make([]int, 2*limit+3)
	var trace [][]int
	for d := 0; d <= limit; d++ {
		trace = append(trace, append([]int(nil), v...))
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
				x = v[off+k+1]
			} else {
				x = v[off+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[off+k] = x
			if x >= n && y >= m {
				backtrack(trace, off, n, m, delA, insB)
				return delA, insB, true
			}
		}
	}
	return nil, nil, false
}

// backtrack walks the saved states back from (n, m) and marks the lines each step removed or added.
func backtrack(trace [][]int, off, n, m int, delA, insB []bool) {
	x, y := n, m
	for d := len(trace) - 1; d >= 0; d-- {
		vd := trace[d]
		k := x - y
		var pk int
		if k == -d || (k != d && vd[off+k-1] < vd[off+k+1]) {
			pk = k + 1
		} else {
			pk = k - 1
		}
		px := vd[off+pk]
		py := px - pk
		mx, my := px+1, py // after a step right: a[px] is removed
		if pk == k+1 {
			mx, my = px, py+1 // after a step down: b[py] is added
		}
		for x > mx && y > my { // back along the run of equal lines
			x--
			y--
		}
		if d > 0 {
			if pk == k+1 {
				insB[py] = true
			} else {
				delA[px] = true
			}
		}
		x, y = px, py
	}
}
