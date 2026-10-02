package tape

import (
	"slices"
	"testing"
)

func TestLines(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"x", []string{"x"}},
		{"x\n", []string{"x"}},
		{"\n", []string{""}},
		{"\n\n", []string{"", ""}},
		{"a\nb\nc", []string{"a", "b", "c"}},
		{"a\nb\nc\n", []string{"a", "b", "c"}},
		{"a\n\nb\n", []string{"a", "", "b"}},
	}
	for _, tt := range tests {
		if got := Lines(tt.in); !slices.Equal(got, tt.want) {
			t.Errorf("Lines(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRangeText(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		start, end int
		want       string
	}{
		{"one line", "a\nb\nc\n", 2, 2, "b"},
		{"two lines", "a\nb\nc\n", 2, 3, "b\nc"},
		{"all", "a\nb\nc\n", 1, 3, "a\nb\nc"},
		{"empty range", "a\nb\nc\n", 2, 1, ""},
		{"empty range at the end", "a\nb\nc\n", 4, 3, ""},
		{"no final newline", "a\nb", 2, 2, "b"},
		{"empty text", "", 1, 0, ""},
		{"blank line", "a\n\nb\n", 2, 2, ""},
		{"past the end is ignored", "a\n", 1, 5, "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RangeText(tt.text, tt.start, tt.end); got != tt.want {
				t.Errorf("RangeText(%q, %d, %d) = %q, want %q", tt.text, tt.start, tt.end, got, tt.want)
			}
		})
	}
}

func TestSplice(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		start, end int
		newText    string
		want       string
	}{
		{"replace one line", "a\nb\nc\n", 2, 2, "B", "a\nB\nc\n"},
		{"replace by more lines", "a\nb\nc\n", 2, 3, "X\nY\nZ", "a\nX\nY\nZ\n"},
		{"replace by fewer lines", "a\nb\nc\n", 1, 3, "x", "x\n"},
		{"delete the first line", "a\nb\nc\n", 1, 1, "", "b\nc\n"},
		{"delete the last line", "a\nb\nc\n", 3, 3, "", "a\nb\n"},
		{"delete everything", "a\nb\n", 1, 2, "", ""},
		{"insert before a line", "a\nb\n", 2, 1, "x", "a\nx\nb\n"},
		{"insert at the top", "a\nb\n", 1, 0, "x", "x\na\nb\n"},
		{"append at the end", "a\nb\nc\n", 4, 3, "d", "a\nb\nc\nd\n"},
		{"write into an empty file", "", 1, 0, "first", "first\n"},
		{"empty replacement of nothing", "", 1, 0, "", ""},
		{"newText with a final newline", "a\nb\n", 2, 2, "x\n", "a\nx\n"},
		{"newText that is one blank line", "a\nb\n", 2, 2, "\n", "a\n\n"},
		{"keeps a missing final newline", "one\ntwo\nthree", 3, 3, "THREE", "one\ntwo\nTHREE"},
		{"append to a file without final newline", "a\nb", 3, 2, "c", "a\nb\nc"},
		{"delete the last line without final newline", "a\nb", 2, 2, "", "a"},
		{"multi-line replacement in the middle", "a\nb\nc\nd\n", 2, 3, "B\nC\nC2", "a\nB\nC\nC2\nd\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Splice(tt.text, tt.start, tt.end, tt.newText); got != tt.want {
				t.Errorf("Splice(%q, %d, %d, %q) = %q, want %q", tt.text, tt.start, tt.end, tt.newText, got, tt.want)
			}
		})
	}
}
