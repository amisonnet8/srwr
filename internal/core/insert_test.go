package core

import (
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/tape"
)

func TestInsertNextToLines(t *testing.T) {
	cases := []struct {
		name    string
		in      EditInput
		want    string
		start   int // the lines that were put in
		end     int
		before  []string
		after   []string
		changed bool // the file is changed by someone after the look
	}{
		{name: "after, by expect", in: EditInput{Expect: str("l2"), NewText: "new", Insert: "after"},
			want: "l1\nl2\nnew\nl3\n", start: 3, end: 3, before: []string{"l1", "l2"}, after: []string{"l3"}},
		{name: "before, by expect", in: EditInput{Expect: str("l2"), NewText: "new", Insert: "before"},
			want: "l1\nnew\nl2\nl3\n", start: 2, end: 2, before: []string{"l1"}, after: []string{"l2", "l3"}},
		{name: "after the last line", in: EditInput{Expect: str("l3"), NewText: "new", Insert: "after"},
			want: "l1\nl2\nl3\nnew\n", start: 4, end: 4, before: []string{"l2", "l3"}, after: []string{}},
		{name: "before the first line", in: EditInput{Expect: str("l1"), NewText: "new", Insert: "before"},
			want: "new\nl1\nl2\nl3\n", start: 1, end: 1, before: []string{}, after: []string{"l1", "l2"}},
		{name: "several lines kept, several put in", in: EditInput{Expect: str("l1\nl2"), NewText: "a\nb", Insert: "after"},
			want: "l1\nl2\na\nb\nl3\n", start: 3, end: 4, before: []string{"l1", "l2"}, after: []string{"l3"}},
		{name: "an empty line", in: EditInput{Expect: str("l1"), NewText: "\n", Insert: "after"},
			want: "l1\n\nl2\nl3\n", start: 2, end: 2, before: []string{"l1"}, after: []string{"l2", "l3"}},
		{name: "an empty newText is an empty line (after)", in: EditInput{Expect: str("l1"), NewText: "", Insert: "after"},
			want: "l1\n\nl2\nl3\n", start: 2, end: 2, before: []string{"l1"}, after: []string{"l2", "l3"}},
		{name: "an empty newText is an empty line (before)", in: EditInput{Expect: str("l2"), NewText: "", Insert: "before"},
			want: "l1\n\nl2\nl3\n", start: 2, end: 2, before: []string{"l1"}, after: []string{"l2", "l3"}},
		{name: "with line numbers", in: EditInput{Expect: str("l3"), NewText: "new", Insert: "after", HasLines: true, StartLine: 3, EndLine: 3},
			want: "l1\nl2\nl3\nnew\n", start: 4, end: 4, before: []string{"l2", "l3"}, after: []string{}},
		{name: "the file changed since the look: expect still finds the lines", changed: true,
			in:   EditInput{Expect: str("l2"), NewText: "new", Insert: "after"},
			want: "l1\nx\nl2\nnew\nl3\n", start: 4, end: 4, before: []string{"x", "l2"}, after: []string{"l3"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.write("f.txt", "l1\nl2\nl3\n")
			if _, err := e.c.Look(LookInput{File: "f.txt", StartLine: 1, EndLine: 3, Why: "w"}); err != nil {
				t.Fatal(err)
			}
			if tc.changed {
				e.write("f.txt", "l1\nx\nl2\nl3\n")
			}
			tc.in.File, tc.in.Why = "f.txt", "w"
			res, err := e.c.Edit(tc.in)
			if err != nil {
				t.Fatalf("err = %+v", err)
			}
			if got := e.read("f.txt"); got != tc.want {
				t.Errorf("file = %q, want %q", got, tc.want)
			}
			if res.StartLine != tc.start || res.EndLine != tc.end {
				t.Errorf("range = %d..%d, want %d..%d", res.StartLine, res.EndLine, tc.start, tc.end)
			}
			if strings.Join(res.Above, "|") != strings.Join(tc.before, "|") || strings.Join(res.Below, "|") != strings.Join(tc.after, "|") {
				t.Errorf("before = %v after = %v", res.Above, res.Below)
			}
			// On the tape it is an edit of an empty range.
			evs := e.events()
			last := evs[len(evs)-1]
			if last.Type != tape.TypeEdit || last.OldText != "" || last.EndLine != last.StartLine-1 {
				t.Errorf("tape event = %+v", last)
			}
		})
	}
}

func TestInsertNextToATokenRange(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "l1\nl2\nl3\n")
	look, err := e.c.Look(LookInput{File: "f.txt", StartLine: 2, EndLine: 2, Why: "w"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.c.Edit(EditInput{Selection: look.Selection, NewText: "new", Insert: "after", Why: "w"})
	if err != nil {
		t.Fatalf("err = %+v", err)
	}
	if got := e.read("f.txt"); got != "l1\nl2\nnew\nl3\n" {
		t.Errorf("file = %q", got)
	}
	// The token returned stands for the new line.
	if _, err := e.c.Edit(EditInput{Selection: res.Selection, NewText: "new2", Why: "w"}); err != nil {
		t.Fatalf("err = %+v", err)
	}
	if got := e.read("f.txt"); got != "l1\nl2\nnew2\nl3\n" {
		t.Errorf("file = %q", got)
	}
}

func TestInsertIsRefused(t *testing.T) {
	cases := []struct {
		name string
		in   EditInput
		code string
	}{
		{"unknown value", EditInput{Expect: str("l2"), NewText: "n", Insert: "next"}, CodeInvalidInput},
		{"empty range", EditInput{NewText: "n", Insert: "after", HasLines: true, StartLine: 2, EndLine: 1}, CodeInvalidInput},
		{"expect not found", EditInput{Expect: str("zz"), NewText: "n", Insert: "after"}, CodeContentNotFound},
		{"expect is part of a line", EditInput{Expect: str("l"), NewText: "n", Insert: "after"}, CodeContentNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.write("f.txt", "l1\nl2\nl3\n")
			tc.in.File, tc.in.Why = "f.txt", "w"
			_, err := e.c.Edit(tc.in)
			wantCode(t, err, tc.code)
			if e.read("f.txt") != "l1\nl2\nl3\n" {
				t.Error("the file was changed")
			}
		})
	}
}

func TestInsertStartAndEnd(t *testing.T) {
	cases := []struct {
		name, file string
		in         EditInput
		want       string
		code       string
	}{
		{name: "end", file: "l1\nl2\n", in: EditInput{NewText: "new", Insert: "end"}, want: "l1\nl2\nnew\n"},
		{name: "start", file: "l1\nl2\n", in: EditInput{NewText: "new", Insert: "start"}, want: "new\nl1\nl2\n"},
		{name: "end of an empty file", file: "", in: EditInput{NewText: "new", Insert: "end"}, want: "new\n"},
		{name: "empty newText is an empty line", file: "l1\n", in: EditInput{Insert: "end"}, want: "l1\n\n"},
		{name: "with expect", file: "l1\n", in: EditInput{NewText: "n", Insert: "end", Expect: str("l1")}, code: CodeInvalidInput},
		{name: "with lines", file: "l1\n", in: EditInput{NewText: "n", Insert: "start", HasLines: true, StartLine: 1, EndLine: 1}, code: CodeInvalidInput},
		{name: "with a token", file: "l1\n", in: EditInput{NewText: "n", Insert: "start", Selection: "sel_x"}, code: CodeInvalidInput},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			e.write("f.txt", c.file)
			c.in.File, c.in.Why = "f.txt", "w"
			_, err := e.c.Edit(c.in)
			if c.code != "" {
				wantCode(t, err, c.code)
				return
			}
			if err != nil {
				t.Fatalf("err = %+v", err)
			}
			if got := e.read("f.txt"); got != c.want {
				t.Errorf("file = %q, want %q", got, c.want)
			}
		})
	}
}

func TestEditsInsertStartAndEnd(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "l1\nl2\n")
	_, err := e.c.Edits(EditsInput{Why: "w", Edits: []EditInput{
		{File: "f.txt", NewText: "top", Insert: "start"},
		{File: "f.txt", NewText: "bottom", Insert: "end"},
	}})
	if err != nil {
		t.Fatalf("err = %+v", err)
	}
	if got := e.read("f.txt"); got != "top\nl1\nl2\nbottom\n" {
		t.Errorf("file = %q", got)
	}
}
