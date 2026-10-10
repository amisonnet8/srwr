package core

import (
	"strings"
	"testing"
)

func TestEditsHint(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", "a1\na2\na3\n")
	e.write("b.txt", "b1\nb2\n")
	edit := func(file, expect, text string) *EditResult {
		t.Helper()
		res, err := e.c.Edit(EditInput{File: file, Expect: str(expect), NewText: text, Why: "w"})
		if err != nil {
			t.Fatalf("err = %+v", err)
		}
		return res
	}
	if h := edit("a.txt", "a1", "A1").Hint; h != "" {
		t.Errorf("the first edit has a hint: %q", h)
	}
	if h := edit("a.txt", "a2", "A2").Hint; h != EditsHint {
		t.Errorf("the second edit of the file: hint = %q", h)
	}
	if h := edit("b.txt", "b1", "B1").Hint; h != "" {
		t.Errorf("an edit of another file has a hint: %q", h)
	}
	if h := edit("a.txt", "a3", "A3").Hint; h != "" {
		t.Errorf("an edit after an edit of another file has a hint: %q", h)
	}
	// Something between them on the tape (a look) ends the run.
	e.sel(e.c, "a.txt", 1, 1)
	if h := edit("a.txt", "A2", "a2").Hint; h != "" {
		t.Errorf("an edit after a look has a hint: %q", h)
	}
	// The hint is not on the tape.
	for _, ev := range e.events() {
		if ev.Why != nil && *ev.Why == EditsHint {
			t.Error("the hint is on the tape")
		}
	}
	// edits (several in one call) is not told about itself.
	edit("a.txt", "A1", "a1")
	if _, err := e.c.Edits(EditsInput{Why: "w", Edits: []EditInput{{File: "a.txt", Expect: str("A3"), NewText: "a3"}}}); err != nil {
		t.Fatal(err)
	}
}

func TestTouchHint(t *testing.T) {
	file := "func a() {\n}\nfunc b() {\n}\n\nvar x = 1\n\t{\"a\", 1},\n"
	cases := []struct {
		name string
		in   EditInput
		want []string // parts the hint holds ("" = no hint)
	}{
		{"a function after a function", EditInput{Expect: str("}"), StartLine: 2, EndLine: 2, HasLines: true, NewText: "func n() {\n}", Insert: "after"}, []string{"touch the line above", "touch the line below"}},
		{"with an empty line on both sides", EditInput{Expect: str("}"), StartLine: 2, EndLine: 2, HasLines: true, NewText: "\nfunc n() {\n}\n\n", Insert: "after"}, nil},
		{"one line is not a block", EditInput{Expect: str("}"), StartLine: 2, EndLine: 2, HasLines: true, NewText: "var y = 2", Insert: "after"}, nil},
		{"another indentation", EditInput{Expect: str("}"), StartLine: 2, EndLine: 2, HasLines: true, NewText: "\tx()\n\ty()", Insert: "after"}, nil},
		{"only below touches", EditInput{Expect: str("var x = 1"), NewText: "\nvar y = 2\nvar z = 3", Insert: "before"}, []string{"touch the line below"}},
		{"elements of a table are one shape", EditInput{Expect: str("\t{\"a\", 1},"), NewText: "\t{\"b\", 2},\n\t{\"c\", 3},", Insert: "after"}, nil},
		{"a function after a } is not one shape", EditInput{Expect: str("}"), StartLine: 4, EndLine: 4, HasLines: true, NewText: "func n() {\n}", Insert: "after"}, []string{"touch the line above"}},
		{"a replacement is not an insertion", EditInput{Expect: str("}"), NewText: "}\n}"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			e.write("f.go", file)
			c.in.File, c.in.Why = "f.go", "w"
			if c.in.Expect != nil && *c.in.Expect == "}" && c.in.StartLine == 0 {
				c.in.StartLine, c.in.EndLine, c.in.HasLines = 2, 2, true
			}
			res, err := e.c.Edit(c.in)
			if err != nil {
				t.Fatalf("err = %+v", err)
			}
			if c.want == nil && res.Hint != "" {
				t.Errorf("hint = %q", res.Hint)
			}
			for _, w := range c.want {
				if !strings.Contains(res.Hint, w) {
					t.Errorf("hint = %q, want %q", res.Hint, w)
				}
			}
		})
	}
}

func TestTouchHintInEdits(t *testing.T) {
	e := newEnv(t)
	e.write("f.go", "func a() {\n}\n")
	res, err := e.c.Edits(EditsInput{Why: "w", Edits: []EditInput{{File: "f.go", NewText: "func b() {\n}", Insert: "end"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Edits[0].Hint, "touch the line above") {
		t.Errorf("hint = %q", res.Edits[0].Hint)
	}
}

func TestEditCutsLongContext(t *testing.T) {
	long := strings.Repeat("x", 300)
	e := newEnv(t)
	e.write("h.md", "# h\n"+long+"\n"+long+"\n")
	res, err := e.c.Edits(EditsInput{Why: "w", Edits: []EditInput{{File: "h.md", NewText: "- new", Insert: "end"}}})
	if err != nil {
		t.Fatal(err)
	}
	r := res.Edits[0]
	if !r.Cut || len(r.Above) != 2 || len([]rune(r.Above[0])) != maxLineLen+1 || !strings.HasSuffix(r.Above[0], "…") {
		t.Errorf("above = %q, cut = %v", r.Above, r.Cut)
	}
	if len(r.Lines) != 1 || r.Lines[0] != "- new" {
		t.Errorf("lines = %q", r.Lines)
	}
	// A single edit of a line that is long itself: lines are not cut; short context is not cut.
	e.write("g.md", "a\n"+long+"\nb\n")
	one, err := e.c.Edit(EditInput{File: "g.md", Expect: str(long), NewText: long + "y", Why: "w"})
	if err != nil {
		t.Fatalf("err = %+v", err)
	}
	if one.Cut || len(one.Lines[0]) != 301 {
		t.Errorf("cut = %v, lines = %d", one.Cut, len(one.Lines[0]))
	}
}
