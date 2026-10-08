package core

import (
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/tape"
)

func (e *env) lookCount() int {
	e.t.Helper()
	n := 0
	for _, ev := range e.events() {
		if ev.Type == tape.TypeLook {
			n++
		}
	}
	return n
}

func TestLooksReadsSeveralFiles(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", "a1\na2\na3\n")
	e.write("dir/b.txt", "b1\nb2\n")
	e.write("empty.txt", "")
	res, err := e.c.Looks(LooksInput{Why: "w", Items: []LooksItem{
		{File: "a.txt", StartLine: 2, EndLine: 9, HasLines: true}, {File: "dir/b.txt"}, {File: "empty.txt"}, {File: "a.txt"}}})
	if err != nil {
		t.Fatalf("err = %+v", err)
	}
	r := res.Items
	if len(r) != 4 || r[0].File != "a.txt" || r[0].StartLine != 2 || r[0].EndLine != 3 || r[0].Note == "" ||
		strings.Join(r[1].Lines, "|") != "b1|b2" || r[1].File != "dir/b.txt" || r[2].EndLine != 0 || len(r[3].Lines) != 3 {
		t.Errorf("items = %+v", r)
	}
	if e.lookCount() != 4 {
		t.Errorf("looks on the tape = %d, want 4", e.lookCount())
	}
	// Every token works with edit.
	for _, it := range r[:2] {
		if _, err := e.c.Edit(EditInput{Selection: it.Selection, NewText: "x", Why: "w"}); err != nil {
			t.Errorf("edit with the token of %s: %+v", it.File, err)
		}
	}
}

func TestLooksIsAllOrNone(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", "a\n")
	e.write(".env", secret)
	big := strings.Repeat("x\n", 1500)
	e.write("big1.txt", big)
	e.write("big2.txt", big)
	if _, err := e.c.Look(LookInput{File: "a.txt", StartLine: 1, EndLine: 1, Why: "w"}); err != nil { // the tape exists
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		items []LooksItem
		code  string
		item  string
	}{
		{"missing file", []LooksItem{{File: "a.txt"}, {File: "nope.txt"}}, CodeFileNotFound, "looks[1]: "},
		{"not recorded", []LooksItem{{File: "a.txt"}, {File: ".env"}}, CodeIgnoredFile, "looks[1]: "},
		{"outside", []LooksItem{{File: "../x"}}, CodeInvalidRange, "looks[0]: "},
		{"start past the end", []LooksItem{{File: "a.txt"}, {File: "a.txt", StartLine: 9, EndLine: 9, HasLines: true}}, CodeInvalidRange, "looks[1]: "},
		{"too many lines", []LooksItem{{File: "big1.txt"}, {File: "big2.txt"}}, CodeInvalidInput, "looks[1]: "},
		{"no items", nil, CodeInvalidInput, "1 to 10"},
		{"eleven items", make([]LooksItem, 11), CodeInvalidInput, "1 to 10"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := len(e.events())
			_, err := e.c.Looks(LooksInput{Why: "w", Items: tc.items})
			wantCode(t, err, tc.code)
			if err != nil && !strings.Contains(err.Message, tc.item) {
				t.Errorf("message = %q, want it to hold %q", err.Message, tc.item)
			}
			for _, ev := range e.events()[before:] {
				if ev.Type == tape.TypeLook || ev.Type == tape.TypeSnapshot {
					t.Errorf("a failed call put %s on the tape", ev.Type)
				}
			}
		})
	}
	_, err := e.c.Looks(LooksInput{Why: " ", Items: []LooksItem{{File: "a.txt"}}})
	wantCode(t, err, CodeInvalidInput)
	e.noContentInFailures("a1", "KEY")
	e.noSecretOnTape(".env")
}
