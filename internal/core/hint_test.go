package core

import "testing"

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
