package trace

import (
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/tape"
)

func str(s string) *string { return &s }

func snap(seq int, file, text string) tape.Event {
	return tape.Event{Type: tape.TypeSnapshot, Seq: seq, TS: "2026-10-10T10:00:00.000Z", File: file, Text: str(text)}
}

func editEv(seq int, ts, file string, start, end int, newText, why string) tape.Event {
	n := len(strings.Split(newText, "\n"))
	return tape.Event{Type: tape.TypeEdit, Seq: seq, TS: ts, File: file, StartLine: start, EndLine: end, NewText: newText,
		NewStartLine: start, NewEndLine: start + n - 1, Selection: str("sel_x"), From: str("sel_y"), Why: str(why), Source: tape.SourceMCP}
}

func lookEv(seq int, file string) tape.Event {
	return tape.Event{Type: tape.TypeLook, Seq: seq, TS: "2026-10-10T10:00:30.000Z", File: file, StartLine: 1, EndLine: 1, Why: str("look"), Selection: str("sel_z"), Source: tape.SourceMCP}
}

// source is a tape of 2 files: a.go is changed 3 times (the 1st and the 3rd are what the commit holds), b.go once.
func source() Source {
	return Source{
		ID:     "20261010-1000-aaaa",
		Header: tape.Event{Type: tape.TypeHeader, Session: "aaaa", StartedAt: "2026-10-10T10:00:00.000Z", Author: &tape.Author{Kind: "ai", Name: "claude"}},
		Events: []tape.Event{
			{Type: tape.TypeHeader, Session: "aaaa", StartedAt: "2026-10-10T10:00:00.000Z", Author: &tape.Author{Kind: "ai", Name: "claude"}},
			snap(1, "a.go", "l1\nl2\nl3\nl4\n"),
			lookEv(2, "a.go"),
			editEv(3, "2026-10-10T10:01:00.000Z", "a.go", 2, 2, "L2\nL2b", "first"),
			snap(4, "b.go", "b1\n"),
			editEv(5, "2026-10-10T10:02:00.000Z", "b.go", 1, 1, "B1", "other file"),
			editEv(6, "2026-10-10T10:03:00.000Z", "a.go", 1, 1, "L1", "between"),
			lookEv(7, "a.go"),
			editEv(8, "2026-10-10T10:04:00.000Z", "a.go", 5, 5, "L4", "third"),
		},
	}
}

func opAt(s Source, si, idx int) *Op {
	e := s.Events[idx]
	return &Op{Tape: s.ID, Seq: e.Seq, Type: e.Type, File: e.File, Source: si, Index: idx}
}

func TestSliceKeepsWhatLiesBetween(t *testing.T) {
	s := source()
	cut, ok := Slice([]Source{s}, []*Op{opAt(s, 0, 3), opAt(s, 0, 8)}, "3f9a1c2d", "v9")
	if !ok {
		t.Fatal("no cut")
	}
	if cut.ID != "20261010-1001-3f9a" {
		t.Errorf("ID = %q (the time of the first operation, the start of the commit)", cut.ID)
	}
	h := cut.Events[0]
	if h.Type != tape.TypeHeader || h.Author == nil || h.Author.Kind != DerivedKind || h.Session != "3f9a" {
		t.Errorf("header = %+v", h)
	}
	var kinds []string
	for i, e := range cut.Events[1:] {
		if e.Seq != i+1 {
			t.Errorf("event %d has seq %d", i, e.Seq)
		}
		if e.Selection != nil || e.From != nil {
			t.Errorf("event %d keeps a token", i)
		}
		if e.File == "b.go" {
			t.Errorf("another file in the cut: %+v", e)
		}
		kinds = append(kinds, e.Type)
	}
	// the snapshot of a.go before the first operation (a.go was not looked at in the cut's range before it), then what lay between.
	if got, want := strings.Join(kinds, " "), "snapshot edit edit look edit"; got != want {
		t.Errorf("kinds = %q, want %q", got, want)
	}
	// Replaying the cut gives the file as the source has it after the last operation.
	got := tape.Build(cut.Events[1:]).Files["a.go"].Text
	want := tape.Build(s.Events[:9]).Files["a.go"].Text
	if got != want {
		t.Errorf("a.go = %q, want %q", got, want)
	}
	if first := cut.Events[1]; first.Type != tape.TypeSnapshot || *first.Text != "l1\nl2\nl3\nl4\n" {
		t.Errorf("snapshot = %+v", first)
	}
}

func TestSliceOfTwoFilesTwoTapesAndANewFile(t *testing.T) {
	s := source()
	n := Source{
		ID:     "20261010-1100-bbbb",
		Header: tape.Event{Type: tape.TypeHeader, Session: "bbbb", StartedAt: "2026-10-10T11:00:00.000Z", Author: &tape.Author{Kind: "ai"}},
		Events: []tape.Event{
			{Type: tape.TypeHeader, Session: "bbbb", StartedAt: "2026-10-10T11:00:00.000Z"},
			{Type: tape.TypeNew, Seq: 1, TS: "2026-10-10T11:00:10.000Z", File: "c.go", StartLine: 1, EndLine: 0, NewText: "c1\nc2",
				NewStartLine: 1, NewEndLine: 2, Why: str("a new file"), Source: tape.SourceMCP},
		},
	}
	srcs := []Source{n, s} // the order of the slice is not the order of the time
	cut, ok := Slice(srcs, []*Op{opAt(n, 0, 1), opAt(s, 1, 5)}, "not a commit, a text", "v9")
	if !ok {
		t.Fatal("no cut")
	}
	var files []string
	for _, e := range cut.Events[1:] {
		files = append(files, e.Type+":"+e.File)
	}
	// The older source first: b.go (snapshot, edit), then the new file (no snapshot: the file was not there).
	if got, want := strings.Join(files, " "), "snapshot:b.go edit:b.go new:c.go"; got != want {
		t.Errorf("events = %q, want %q", got, want)
	}
	st := tape.Build(cut.Events[1:])
	if strings.TrimSpace(st.Files["c.go"].Text) != "c1\nc2" || strings.TrimSpace(st.Files["b.go"].Text) != "B1" {
		t.Errorf("files = %+v %+v", st.Files["c.go"], st.Files["b.go"])
	}
}

func TestSliceWithNothing(t *testing.T) {
	if _, ok := Slice([]Source{source()}, nil, "x", "v"); ok {
		t.Error("a cut of no operations")
	}
}
