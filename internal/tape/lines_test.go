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

func TestSpliceLines(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		start, end int
		lines      []string
		want       string
	}{
		{"blank last line", "a\nb\n", 2, 2, []string{"x", ""}, "a\nx\n\n"},
		{"one blank line", "a\nb\n", 2, 2, []string{""}, "a\n\n"},
		{"two blank lines", "a\nb\n", 1, 1, []string{"", ""}, "\n\nb\n"},
		{"no lines deletes", "a\nb\n", 2, 2, nil, "a\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SpliceLines(tt.text, tt.start, tt.end, tt.lines); got != tt.want {
				t.Errorf("SpliceLines = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewLines(t *testing.T) {
	tests := []struct {
		name string
		e    Event
		want []string
	}{
		{"deletion", Event{NewText: "", NewStartLine: 3, NewEndLine: 2}, nil},
		{"one blank line", Event{NewText: "", NewStartLine: 3, NewEndLine: 3}, []string{""}},
		{"ends in a blank line", Event{NewText: "a\n", NewStartLine: 3, NewEndLine: 4}, []string{"a", ""}},
		{"two blank lines", Event{NewText: "\n", NewStartLine: 1, NewEndLine: 2}, []string{"", ""}},
		{"plain", Event{NewText: "a\nb", NewStartLine: 1, NewEndLine: 2}, []string{"a", "b"}},
		{"old tape: text ends with a newline", Event{NewText: "a\n", NewStartLine: 1, NewEndLine: 1}, []string{"a"}},
		{"old tape: no new range", Event{NewText: "a\nb\n"}, []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewLines(tt.e); !slices.Equal(got, tt.want) {
				t.Errorf("NewLines = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStateKeepsBlankLines(t *testing.T) {
	st := Build([]Event{
		{Type: TypeSnapshot, Seq: 1, File: "a.go", Text: Str("a\nb\n")},
		{Type: TypeReplace, Seq: 2, File: "a.go", StartLine: 2, EndLine: 2, NewText: "x\n", NewStartLine: 2, NewEndLine: 3},
		{Type: TypeReplace, Seq: 3, File: "a.go", StartLine: 1, EndLine: 0, NewText: "", NewStartLine: 1, NewEndLine: 1},
	})
	if got, want := st.Files["a.go"].Text, "\na\nx\n\n"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}
