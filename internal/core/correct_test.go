package core

import (
	"slices"
	"testing"

	"github.com/amisonnet8/srwr/internal/tape"
)

// edit makes a replace event that put newLines lines in place of start..end.
func edit(seq int, file string, start, end, newLines int) tape.Event {
	return tape.Event{
		Type: tape.TypeEdit, Seq: seq, File: file, StartLine: start, EndLine: end,
		NewStartLine: start, NewEndLine: start + newLines - 1, NewText: join(newLines),
	}
}

func join(n int) string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = "x"
	}
	return joinLines(lines)
}

func joinLines(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}

func TestCorrect(t *testing.T) {
	// The token is for lines 10..12 unless the case says otherwise, issued at seq 5.
	tests := []struct {
		name         string
		replaces     []tape.Event
		a, b         int
		wantA, wantB int
		wantOK       bool
	}{
		{"no edits", nil, 10, 12, 10, 12, true},
		{"edit above adds lines", []tape.Event{edit(6, "f", 2, 3, 5)}, 10, 12, 13, 15, true},
		{"edit above removes lines", []tape.Event{edit(6, "f", 2, 5, 1)}, 10, 12, 7, 9, true},
		{"edit above same size", []tape.Event{edit(6, "f", 2, 3, 2)}, 10, 12, 10, 12, true},
		{"edit above deletes", []tape.Event{edit(6, "f", 2, 3, 0)}, 10, 12, 8, 10, true},
		{"edit just above (ends at a-1)", []tape.Event{edit(6, "f", 8, 9, 4)}, 10, 12, 12, 14, true},
		{"edit just below (starts at b+1)", []tape.Event{edit(6, "f", 13, 14, 9)}, 10, 12, 10, 12, true},
		{"edit far below", []tape.Event{edit(6, "f", 40, 50, 1)}, 10, 12, 10, 12, true},
		{"insertion above", []tape.Event{edit(6, "f", 3, 2, 2)}, 10, 12, 12, 14, true},
		{"insertion right before the range", []tape.Event{edit(6, "f", 10, 9, 2)}, 10, 12, 12, 14, true},
		{"insertion right after the range", []tape.Event{edit(6, "f", 13, 12, 2)}, 10, 12, 10, 12, true},
		{"insertion inside the range", []tape.Event{edit(6, "f", 11, 10, 2)}, 10, 12, 10, 12, false},
		{"insertion after the first line of the range", []tape.Event{edit(6, "f", 11, 10, 1)}, 10, 12, 10, 12, false},
		{"edit overlapping the top", []tape.Event{edit(6, "f", 8, 10, 1)}, 10, 12, 10, 12, false},
		{"edit overlapping the bottom", []tape.Event{edit(6, "f", 12, 14, 1)}, 10, 12, 10, 12, false},
		{"edit inside the range", []tape.Event{edit(6, "f", 11, 11, 3)}, 10, 12, 10, 12, false},
		{"edit around the range", []tape.Event{edit(6, "f", 9, 13, 1)}, 10, 12, 10, 12, false},
		{"the same lines", []tape.Event{edit(6, "f", 10, 12, 3)}, 10, 12, 10, 12, false},
		{"another file is ignored", []tape.Event{edit(6, "g", 10, 12, 1)}, 10, 12, 10, 12, true},
		{"an edit at or before the token's seq is ignored", []tape.Event{edit(4, "f", 10, 12, 1), edit(5, "f", 1, 1, 9)}, 10, 12, 10, 12, true},
		{"edits apply in order", []tape.Event{edit(6, "f", 1, 1, 3), edit(7, "f", 20, 20, 1), edit(8, "f", 1, 2, 1)}, 10, 12, 11, 13, true},
		{"the range moves, then the next edit is judged where it is", []tape.Event{edit(6, "f", 1, 1, 6), edit(7, "f", 16, 16, 2)}, 10, 12, 15, 17, false},
		{"moves again", []tape.Event{edit(6, "f", 1, 1, 6), edit(7, "f", 1, 1, 1)}, 10, 12, 15, 17, true},
		// An empty range is the place before line a.
		{"empty range, edit above", []tape.Event{edit(6, "f", 1, 2, 5)}, 10, 9, 13, 12, true},
		{"empty range, edit below", []tape.Event{edit(6, "f", 10, 11, 5)}, 10, 9, 10, 9, true},
		{"empty range, insertion at the same place", []tape.Event{edit(6, "f", 10, 9, 2)}, 10, 9, 12, 11, true},
		{"empty range, edit covering the place", []tape.Event{edit(6, "f", 9, 10, 1)}, 10, 9, 10, 9, false},
		{"empty range at the end, lines appended", []tape.Event{edit(6, "f", 10, 9, 3)}, 10, 9, 13, 12, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b, ok := Correct(tt.replaces, "f", 5, tt.a, tt.b)
			if ok != tt.wantOK || a != tt.wantA || b != tt.wantB {
				t.Errorf("Correct = %d..%d ok=%v, want %d..%d ok=%v", a, b, ok, tt.wantA, tt.wantB, tt.wantOK)
			}
		})
	}
}

func TestRangeLines(t *testing.T) {
	text := "a\nb\nc\n"
	tests := []struct {
		a, b int
		want []string
	}{
		{1, 3, []string{"a", "b", "c"}},
		{2, 2, []string{"b"}},
		{2, 1, []string{}},
		{4, 3, []string{}},
		{0, 2, []string{"a", "b"}},
		{2, 9, []string{"b", "c"}},
		{9, 12, []string{}},
	}
	for _, tt := range tests {
		got := rangeLines(text, tt.a, tt.b)
		if got == nil || !slices.Equal(got, tt.want) {
			t.Errorf("rangeLines(%d, %d) = %#v, want %#v", tt.a, tt.b, got, tt.want)
		}
	}
}
