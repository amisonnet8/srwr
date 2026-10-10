package trace

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/amisonnet8/srwr/internal/tape"
)

// Op is one edit or new of a tape: the lines it wrote and why.
type Op struct {
	Tape  string
	Seq   int
	Type  string // "edit" or "new"
	File  string
	Time  time.Time
	Why   string
	Lines []string
}

// Collect reads the tapes of dir (only, when non-empty) and returns their edits and news, oldest first. A tape that cannot be
// read is skipped.
func Collect(dir, only string) []Op {
	ids := tape.IDs(dir)
	if only != "" {
		ids = []string{only}
	}
	var ops []Op
	for _, id := range ids {
		path, ok := tape.Find(dir, id)
		if !ok {
			continue
		}
		b, err := tape.ReadFile(path)
		if err != nil {
			continue
		}
		for _, e := range tape.Parse(b).Events {
			if (e.Type != tape.TypeEdit && e.Type != tape.TypeNew) || e.File == "" {
				continue
			}
			op := Op{Tape: id, Seq: e.Seq, Type: e.Type, File: filepath.ToSlash(e.File), Lines: tape.Lines(e.NewText)}
			if e.Why != nil {
				op.Why = *e.Why
			}
			op.Time, _ = time.Parse(time.RFC3339, e.TS)
			ops = append(ops, op)
		}
	}
	sort.SliceStable(ops, func(i, j int) bool {
		if !ops[i].Time.Equal(ops[j].Time) {
			return ops[i].Time.Before(ops[j].Time)
		}
		if ops[i].Tape != ops[j].Tape {
			return ops[i].Tape < ops[j].Tape
		}
		return ops[i].Seq < ops[j].Seq
	})
	return ops
}

// Segment is added lines From..To (line numbers in the new file) that Op wrote, or that no operation did (Op is nil).
type Segment struct {
	From, To int
	Op       *Op
	Hunk     int // index of the hunk in the FileDiff
}

// minOneLine is how many characters a single matching line needs (trimmed) to count by itself; a shorter line ("}") is only
// believed in a run of two or more.
const minOneLine = 12

// Attribute finds the operations that wrote the added lines of a file. ops are oldest first, and a later one wins a tie.
func Attribute(ops []Op, f FileDiff) []Segment {
	var mine []*Op
	for i := range ops {
		if ops[i].File == f.Path {
			mine = append(mine, &ops[i])
		}
	}
	var segs []Segment
	for hi, h := range f.Hunks {
		for _, r := range h.Runs {
			segs = append(segs, split(mine, r.Lines, r.Start, hi)...)
		}
	}
	sort.SliceStable(segs, func(i, j int) bool { return segs[i].From < segs[j].From })
	return segs
}

// split attributes lines (the first is line number start) by the longest run of them that some operation wrote, then the
// rest on each side of it.
func split(ops []*Op, lines []string, start, hunk int) []Segment {
	if len(lines) == 0 {
		return nil
	}
	bestLen, bestAt, bestOp := 0, 0, (*Op)(nil)
	for _, op := range ops { // oldest first: >= lets a later operation win
		l, at := longestCommon(lines, op.Lines)
		if l > 0 && l >= bestLen {
			bestLen, bestAt, bestOp = l, at, op
		}
	}
	if bestOp == nil || (bestLen == 1 && len(strings.TrimSpace(lines[bestAt])) < minOneLine) {
		return []Segment{{From: start, To: start + len(lines) - 1, Hunk: hunk}}
	}
	out := split(ops, lines[:bestAt], start, hunk)
	out = append(out, Segment{From: start + bestAt, To: start + bestAt + bestLen - 1, Op: bestOp, Hunk: hunk})
	return append(out, split(ops, lines[bestAt+bestLen:], start+bestAt+bestLen, hunk)...)
}

// longestCommon returns the length of the longest stretch of a that b holds as it is, and where it starts in a.
func longestCommon(a, b []string) (length, at int) {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for i := range a {
		for j := range b {
			if a[i] == b[j] {
				cur[j+1] = prev[j] + 1
				if cur[j+1] > length {
					length, at = cur[j+1], i-cur[j+1]+1
				}
			} else {
				cur[j+1] = 0
			}
		}
		prev, cur = cur, prev
	}
	return length, at
}
