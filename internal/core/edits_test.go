package core

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/tape"
)

func countType(evs []tape.Event, typ string) int {
	n := 0
	for _, ev := range evs {
		if ev.Type == typ {
			n++
		}
	}
	return n
}

// Several edits of one file, given in any order, are found as the file was before the call and made together.
func TestEditsOneFile(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "l1\nl2\nl3\nl4\nl5\nl6\nl7\n")
	look := e.sel(e.c, "f.txt", 5, 5)
	res, err := e.c.Edits(EditsInput{Why: "w", Edits: []EditInput{
		{File: "f.txt", Expect: str("l6"), NewText: "six"},                                        // 0: replace
		{Selection: look.Selection, NewText: "five\nfive+", Insert: ""},                           // 1: a token, grows by one line
		{File: "f.txt", Expect: str("l2"), NewText: "after2", Insert: InsertAfter},                // 2: insert
		{File: "f.txt", HasLines: true, StartLine: 4, EndLine: 4, Expect: str("l4"), NewText: ""}, // 3: delete, by line numbers before the call
		{File: "f.txt", Expect: str("l1"), NewText: "one\nuno", Insert: ""},                       // 4: grows by one line
		{File: "f.txt", Expect: str("l7"), NewText: "before7", Insert: InsertBefore},              // 5: insert next to the one after
	}})
	if err != nil {
		t.Fatalf("err = %+v", err)
	}
	want := "one\nuno\nl2\nafter2\nl3\nfive\nfive+\nsix\nbefore7\nl7\n"
	if got := e.read("f.txt"); got != want {
		t.Fatalf("file = %q, want %q", got, want)
	}
	// The results are in the order given, with the lines of the file after the call.
	wantLines := [][]string{{"six"}, {"five", "five+"}, {"after2"}, {}, {"one", "uno"}, {"before7"}}
	wantStart := []int{8, 6, 4, 5, 1, 9}
	for i, r := range res.Edits {
		if !slices.Equal(r.Lines, wantLines[i]) || (len(wantLines[i]) > 0 && r.StartLine != wantStart[i]) {
			t.Errorf("edits[%d] = %+v, want lines %v at %d", i, r, wantLines[i], wantStart[i])
		}
	}
	if !slices.Equal(res.Edits[0].Above, []string{"five", "five+"}) || !slices.Equal(res.Edits[0].Below, []string{"before7", "l7"}) {
		t.Errorf("above and below = %v %v", res.Edits[0].Above, res.Edits[0].Below)
	}
	// One edit on the tape for each item, with the same why, and the replay is the file.
	evs := e.events()
	if n := countType(evs, tape.TypeEdit); n != 6 {
		t.Errorf("edits on the tape = %d", n)
	}
	for _, ev := range evs {
		if ev.Type == tape.TypeEdit && (ev.Why == nil || *ev.Why != "w") {
			t.Errorf("edit = %+v", ev)
		}
	}
	e.checkTape()
	// Every token that came back stands for its lines.
	for i, r := range res.Edits {
		if i == 3 {
			continue // a deletion: the range is empty
		}
		if _, err := e.c.Edit(EditInput{Selection: r.Selection, NewText: strings.Join(r.Lines, "\n") + "!", Why: "w"}); err != nil {
			t.Errorf("edits[%d]: token does not work: %+v", i, err)
		}
	}
	e.checkTape()
}

func TestEditsSeveralFiles(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", "a1\na2\n")
	e.write("b.txt", "b1\nb2\n")
	look := e.sel(e.c, "b.txt", 1, 1)
	res, err := e.c.Edits(EditsInput{Why: "w", Edits: []EditInput{
		{File: "a.txt", Expect: str("a2"), NewText: "A2"},
		{Selection: look.Selection, NewText: "B1"},
		{File: "a.txt", Expect: str("a1"), NewText: "A1"},
	}})
	if err != nil {
		t.Fatalf("err = %+v", err)
	}
	if e.read("a.txt") != "A1\nA2\n" || e.read("b.txt") != "B1\nb2\n" || len(res.Edits) != 3 {
		t.Errorf("a = %q b = %q", e.read("a.txt"), e.read("b.txt"))
	}
	e.checkTape()
}

// Nothing is changed when a call is refused, and the tape holds a failure, not edits.
func TestEditsAreAllOrNone(t *testing.T) {
	cases := []struct {
		name  string
		edits []EditInput
		code  string
		text  string // part of the message
	}{
		{"an item that does not match", []EditInput{{File: "f.txt", Expect: str("l1"), NewText: "x"}, {File: "f.txt", Expect: str("zz"), NewText: "y"}},
			CodeContentNotFound, "edits[1]: "},
		{"an ambiguous item", []EditInput{{File: "f.txt", Expect: str("l1"), NewText: "x"}, {File: "f.txt", Expect: str("dup"), NewText: "y"}},
			CodeContentAmbiguous, "edits[1]: "},
		{"the same lines twice", []EditInput{{File: "f.txt", Expect: str("l1"), NewText: "x"}, {File: "f.txt", Expect: str("l1"), NewText: "y"}},
			CodeInvalidInput, "edits[0] and edits[1] overlap"},
		{"ranges that share a line", []EditInput{{File: "f.txt", Expect: str("l1\nl2"), NewText: "x"}, {File: "f.txt", Expect: str("l2\nl3"), NewText: "y"}},
			CodeInvalidInput, "overlap"},
		{"an insertion inside a range", []EditInput{{File: "f.txt", Expect: str("l1\nl2\nl3"), NewText: "x"}, {File: "f.txt", Expect: str("l1"), NewText: "y", Insert: InsertAfter}},
			CodeInvalidInput, "overlap"},
		{"two insertions at one place", []EditInput{{File: "f.txt", Expect: str("l1"), NewText: "x", Insert: InsertAfter}, {File: "f.txt", Expect: str("l2"), NewText: "y", Insert: InsertBefore}},
			CodeInvalidInput, "overlap"},
		{"CR in an item", []EditInput{{File: "f.txt", Expect: str("l1"), NewText: "x"}, {File: "f.txt", Expect: str("l2"), NewText: "y\r"}},
			CodeInvalidInput, "edits[1]: "},
		{"a bad token", []EditInput{{Selection: "sel_0000", NewText: "x"}}, CodeInvalidSelection, "edits[0]: "},
		{"no edits", nil, CodeInvalidInput, "1 to 50"},
		{"too many edits", slices.Repeat([]EditInput{{File: "f.txt", Expect: str("l1"), NewText: "x"}}, maxEdits+1), CodeInvalidInput, "1 to 50"},
		{"a file that is not recorded", []EditInput{{File: ".env", Expect: str("SECRET=1"), NewText: "x"}}, CodeIgnoredFile, "edits[0]: "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.write("f.txt", "l1\nl2\nl3\ndup\ndup\n")
			e.write(".env", "SECRET=1\n")
			_, err := e.c.Edits(EditsInput{Why: "w", Edits: tc.edits})
			if err == nil || err.Code != tc.code || !strings.Contains(err.Message, tc.text) {
				t.Fatalf("err = %+v, want %s with %q", err, tc.code, tc.text)
			}
			if got := e.read("f.txt"); got != "l1\nl2\nl3\ndup\ndup\n" {
				t.Errorf("file = %q: it must not change", got)
			}
			evs := e.events()
			if countType(evs, tape.TypeEdit) != 0 || countType(evs, tape.TypeFailure) != 1 {
				t.Errorf("tape = %v", e.kinds())
			}
			for _, ev := range evs {
				if ev.Type == tape.TypeFailure && (strings.Contains(ev.Failure.Message, "SECRET") || ev.Failure.Tool != "edit") {
					t.Errorf("failure = %+v", ev.Failure)
				}
			}
		})
	}
}

func TestEditsNeedAWhy(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "l1\n")
	if _, err := e.c.Edits(EditsInput{Why: " ", Edits: []EditInput{{File: "f.txt", Expect: str("l1"), NewText: "x"}}}); err == nil || err.Code != CodeInvalidInput {
		t.Errorf("err = %+v", err)
	}
}

// One edit in edits does what edit does: the tape holds the same event.
func TestEditsOfOneAreLikeEdit(t *testing.T) {
	var events [2]tape.Event
	for i := range events {
		e := newEnv(t)
		e.write("f.txt", "l1\nl2\n")
		item := EditInput{File: "f.txt", Expect: str("l2"), NewText: "x\ny", Insert: InsertAfter}
		if i == 0 {
			item.Why = "w"
			if _, err := e.c.Edit(item); err != nil {
				t.Fatal(err)
			}
		} else if _, err := e.c.Edits(EditsInput{Why: "w", Edits: []EditInput{item}}); err != nil {
			t.Fatal(err)
		}
		evs := e.events()
		events[i] = evs[len(evs)-1]
	}
	a, b := events[0], events[1]
	a.TS, b.TS, a.Selection, b.Selection = "", "", nil, nil
	if a.File != b.File || a.StartLine != b.StartLine || a.EndLine != b.EndLine || a.OldText != b.OldText || a.NewText != b.NewText ||
		a.NewStartLine != b.NewStartLine || a.NewEndLine != b.NewEndLine || a.FileShaBefore != b.FileShaBefore || a.FileShaAfter != b.FileShaAfter {
		t.Errorf("edit = %+v\nedits = %+v", a, b)
	}
}

// An item with content makes a file, in the same call that changes the files that use it.
func TestEditsMakeFiles(t *testing.T) {
	e := newEnv(t)
	e.write("main.txt", "use nothing\nend\n")
	res, err := e.c.Edits(EditsInput{Why: "w", Edits: []EditInput{
		{File: "main.txt", Expect: str("use nothing"), NewText: "use extra"},
		{File: "pkg/extra.txt", Create: str("one\ntwo")},
	}})
	if err != nil {
		t.Fatalf("err = %+v", err)
	}
	if e.read("main.txt") != "use extra\nend\n" || e.read("pkg/extra.txt") != "one\ntwo\n" {
		t.Errorf("files = %q %q", e.read("main.txt"), e.read("pkg/extra.txt"))
	}
	if r := res.Edits[1]; r.StartLine != 1 || r.EndLine != 2 || !slices.Equal(r.Lines, []string{"one", "two"}) || r.Selection == "" || res.Edits[0].Selection == "" {
		t.Errorf("results = %+v", res.Edits)
	}
	evs := e.events()
	if countType(evs, tape.TypeNew) != 1 || countType(evs, tape.TypeEdit) != 1 {
		t.Errorf("new = %d, edit = %d", countType(evs, tape.TypeNew), countType(evs, tape.TypeEdit))
	}
	// The token of the made file works with edit.
	if _, err := e.c.Edit(EditInput{Selection: res.Edits[1].Selection, NewText: "x", Why: "w"}); err != nil {
		t.Errorf("edit with the token: %+v", err)
	}
	e.checkTape()
}

func TestEditsMakeFilesAllOrNone(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", "a\n")
	e.write("have.txt", "have\n")
	e.write(".env", "x\n")
	cases := []struct {
		name  string
		items []EditInput
		code  string
		word  string
	}{
		{"the file exists", []EditInput{{File: "a.txt", Expect: str("a"), NewText: "A"}, {File: "have.txt", Create: str("x")}}, CodeFileExists, "edits[1]: "},
		{"not recorded", []EditInput{{File: ".env", Create: str("x")}, {File: "a.txt", Expect: str("a"), NewText: "A"}}, CodeIgnoredFile, "edits[0]: "},
		{"outside", []EditInput{{File: "../x.txt", Create: str("x")}}, CodeInvalidRange, "edits[0]: "},
		{"with other inputs", []EditInput{{File: "n.txt", Create: str("x"), NewText: "y"}}, CodeInvalidInput, "file and content only"},
		{"with lines", []EditInput{{File: "n.txt", Create: str("x"), HasLines: true, StartLine: 1, EndLine: 1}}, CodeInvalidInput, "file and content only"},
		{"no file", []EditInput{{Create: str("x")}}, CodeInvalidInput, "give file"},
		{"CR", []EditInput{{File: "n.txt", Create: str("a\r\n")}}, CodeInvalidInput, "CR"},
		{"twice", []EditInput{{File: "n.txt", Create: str("x")}, {File: "n.txt", Create: str("y")}}, CodeInvalidInput, "makes n.txt as well"},
		{"a later item fails", []EditInput{{File: "n.txt", Create: str("x")}, {File: "a.txt", Expect: str("zz"), NewText: "A"}}, CodeContentNotFound, "edits[1]: "},
		{"edit of a file that is made", []EditInput{{File: "n.txt", Create: str("x")}, {File: "n.txt", Expect: str("x"), NewText: "y"}}, CodeFileNotFound, "edits[1]: "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.c.Edits(EditsInput{Why: "w", Edits: tc.items})
			wantCode(t, err, tc.code)
			if err != nil && !strings.Contains(err.Message, tc.word) {
				t.Errorf("message = %q, want it to hold %q", err.Message, tc.word)
			}
			if e.read("a.txt") != "a\n" || e.read("have.txt") != "have\n" {
				t.Error("a file was changed")
			}
			for _, f := range []string{"n.txt", "x.txt"} {
				if _, serr := os.Stat(filepath.Join(e.root, f)); serr == nil {
					t.Errorf("%s was made", f)
				}
			}
		})
	}
	e.noSecretOnTape(".env")
}
