package core

import (
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/tape"
)

// editAt edits a file by its path; lines is nil, or the line numbers as the client saw them.
func (e *env) editAt(file string, lines []int, expect *string, newText string) (*EditResult, *Error) {
	in := EditInput{File: file, Expect: expect, NewText: newText, Why: "change it"}
	if lines != nil {
		in.HasLines, in.StartLine, in.EndLine = true, lines[0], lines[1]
	}
	return e.c.Edit(in)
}

func str(s string) *string { return &s }

const tenLines = "l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\nl10\n"

func TestEditByFileFindsTheRange(t *testing.T) {
	cases := []struct {
		name   string
		prep   func(e *env)
		lines  []int
		expect *string
		new    string
		want   string
		code   string
	}{
		{name: "line numbers that hold expect", lines: []int{3, 4}, expect: str("l3\nl4"), new: "X", want: "l1\nl2\nX\nl5\nl6\nl7\nl8\nl9\nl10\n"},
		{name: "expect alone, in one place", expect: str("l6"), new: "X\nY", want: "l1\nl2\nl3\nl4\nl5\nX\nY\nl7\nl8\nl9\nl10\n"},
		{name: "wrong line numbers, expect in one place", lines: []int{2, 2}, expect: str("l7"), new: "X", want: "l1\nl2\nl3\nl4\nl5\nl6\nX\nl8\nl9\nl10\n"},
		{name: "delete", lines: []int{2, 3}, expect: str("l2\nl3"), new: "", want: "l1\nl4\nl5\nl6\nl7\nl8\nl9\nl10\n"},
		{name: "insert, after a look", prep: func(e *env) { e.sel(e.c, "f.txt", 1, 2) }, lines: []int{3, 2}, new: "new", want: "l1\nl2\nnew\nl3\nl4\nl5\nl6\nl7\nl8\nl9\nl10\n"},
		{name: "insert at the end", prep: func(e *env) { e.sel(e.c, "f.txt", 1, 2) }, lines: []int{11, 10}, new: "end", want: tenLines + "end\n"},
		{name: "insert, never looked at", lines: []int{3, 2}, new: "new", code: CodeContentNotFound},
		{
			name: "insert, after the file moved",
			prep: func(e *env) {
				s := e.sel(e.c, "f.txt", 1, 1)
				e.rep(e.c, s.Selection, "one")
			},
			lines: []int{3, 2}, new: "new", code: CodeContentNotFound,
		},
		{name: "insert with expect", lines: []int{3, 2}, expect: str("l3"), new: "new", code: CodeInvalidInput},
		{name: "insert out of range", prep: func(e *env) { e.sel(e.c, "f.txt", 1, 2) }, lines: []int{12, 11}, new: "new", code: CodeInvalidRange},
		{name: "no expect", lines: []int{3, 4}, new: "X", code: CodeInvalidInput},
		{name: "expect not in the file", expect: str("zzz"), new: "X", code: CodeContentNotFound},
		{name: "expect in two places", prep: func(e *env) { e.write("f.txt", "a\nb\na\nb\n") }, expect: str("a\nb"), new: "X", code: CodeContentAmbiguous},
		{name: "expect in two places, line numbers tell", prep: func(e *env) { e.write("f.txt", "a\nb\na\nb\n") }, lines: []int{3, 4}, expect: str("a\nb"), new: "X", want: "a\nb\nX\n"},
		{name: "line numbers out of range", lines: []int{9, 14}, expect: str("l9"), new: "X", code: CodeInvalidRange},
		{name: "expect with CR", expect: str("l1\r"), new: "X", code: CodeInvalidInput},
		{name: "whitespace counts", expect: str("l1 "), new: "X", code: CodeContentNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.write("f.txt", tenLines)
			if tc.prep != nil {
				tc.prep(e)
			}
			before := e.read("f.txt")
			res, err := e.editAt("f.txt", tc.lines, tc.expect, tc.new)
			if tc.code != "" {
				wantCode(t, err, tc.code)
				if e.read("f.txt") != before {
					t.Error("the file was changed")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := e.read("f.txt"); got != tc.want {
				t.Errorf("file = %q, want %q", got, tc.want)
			}
			if res.Selection == "" || len(res.Lines) != len(tape.Lines(tc.new)) {
				t.Errorf("result = %+v", res)
			}
			e.checkTape()
		})
	}
}

// Edits to one file sent together, as parallel calls are: each is found by its lines, wherever the earlier ones moved them.
func TestEditByFileAfterEditsAbove(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", tenLines)
	e.sel(e.c, "f.txt", 1, 10) // the client saw the file once; the line numbers below are those of that look
	steps := []struct {
		lines  []int
		expect string
		new    string
	}{
		{[]int{2, 2}, "l2", "A\nA2\nA3"}, // 2 lines more
		{[]int{5, 5}, "l5", "B"},         // now at line 7
		{[]int{9, 9}, "l9", ""},          // now at line 11, and the file is shorter
		{[]int{1, 1}, "l1", "C"},
	}
	for _, s := range steps {
		if _, err := e.editAt("f.txt", s.lines, str(s.expect), s.new); err != nil {
			t.Fatalf("%v: %v", s.lines, err)
		}
	}
	if got, want := e.read("f.txt"), "C\nA\nA2\nA3\nl3\nl4\nB\nl6\nl7\nl8\nl10\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
	// Every edit is one edit event without a token it came from.
	edits := e.eventsOf(tape.TypeEdit)
	if len(edits) != 4 {
		t.Fatalf("%d edits", len(edits))
	}
	for _, ev := range edits {
		if ev.From != nil || ev.Source != tape.SourceMCP || ev.Selection == nil {
			t.Errorf("edit = %+v", ev)
		}
	}
	e.checkTape()
}

// The look that counts may be a hook's: the client read the file with Read.
func TestEditByFileCorrectsFromAHookRead(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", tenLines)
	e.hook(HookRequest{Looks: []HookLook{{File: "f.txt", Range: HookRange{Mode: RangeAll}, Tool: "Read"}}})
	if _, err := e.editAt("f.txt", []int{2, 2}, str("l2"), "A\nB"); err != nil {
		t.Fatal(err)
	}
	// l8 is at line 9 now; the client still says 8. The file also has l9 twice, so only the corrected number is right.
	if _, err := e.editAt("f.txt", []int{8, 8}, str("l8"), "X"); err != nil {
		t.Fatal(err)
	}
	if got, want := e.read("f.txt"), "l1\nA\nB\nl3\nl4\nl5\nl6\nl7\nX\nl9\nl10\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
}

// The line numbers as given hold expect in one place, and the corrected ones in another: it is not clear which is meant.
func TestEditByFileAmbiguousBetweenGivenAndCorrected(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "x\nx\nx\nx\n")
	e.sel(e.c, "f.txt", 1, 4)
	if _, err := e.editAt("f.txt", []int{1, 1}, str("x"), "x\nx"); err != nil { // lines move down by one
		t.Fatal(err)
	}
	_, err := e.editAt("f.txt", []int{3, 3}, str("x"), "Y")
	wantCode(t, err, CodeContentAmbiguous)
	if !strings.Contains(err.Message, "places") {
		t.Errorf("message = %q", err.Message)
	}
}

// When an edit overlaps the range, the correction stops; the one place where expect is still finds it.
func TestEditByFileFallsBackToSearch(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", tenLines)
	s := e.sel(e.c, "f.txt", 1, 10)
	e.rep(e.c, s.Selection, "l1\nl2\nl3\nlX\nl5\nl6\nl7\nl8\nl9\nl10") // overlaps every range
	if _, err := e.editAt("f.txt", []int{9, 9}, str("l9"), "N"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.read("f.txt"), "l8\nN\nl10") {
		t.Errorf("file = %q", e.read("f.txt"))
	}
}

func TestEditInputIsChecked(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "a\n")
	s := e.sel(e.c, "f.txt", 1, 1)
	for name, in := range map[string]EditInput{
		"neither":       {NewText: "x", Why: "w"},
		"both":          {Selection: s.Selection, File: "f.txt", NewText: "x", Why: "w"},
		"token, lines":  {Selection: s.Selection, HasLines: true, StartLine: 1, EndLine: 1, NewText: "x", Why: "w"},
		"token, expect": {Selection: s.Selection, Expect: str("a"), NewText: "x", Why: "w"},
	} {
		_, err := e.c.Edit(in)
		if err == nil || err.Code != CodeInvalidInput {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if e.read("f.txt") != "a\n" {
		t.Error("the file was changed")
	}
}

func TestEditByFileKeepsFilesOutOfTheTape(t *testing.T) {
	e := newEnv(t)
	e.write(".env", secret)
	e.write("f.txt", "a\n")
	_, err := e.editAt(".env", nil, str(secret), "x")
	wantCode(t, err, CodeIgnoredFile)
	_, err = e.editAt("../x.txt", nil, str("a"), "x")
	wantCode(t, err, CodeInvalidRange)
	_, err = e.editAt("nope.txt", nil, str("a"), "x")
	wantCode(t, err, CodeFileNotFound)

	// A failure on the tape has the file and the line numbers, never expect or newText.
	_, err = e.editAt("f.txt", []int{1, 1}, str("EXPECTSECRET"), "NEWSECRET")
	wantCode(t, err, CodeContentNotFound)
	f := e.failures()
	last := f[len(f)-1]
	if last.Tool != "edit" || last.File == nil || *last.File != "f.txt" || last.StartLine == nil || *last.StartLine != 1 || last.Selection != nil {
		t.Errorf("failure = %+v", last)
	}
	e.noSecretOnTape("EXPECTSECRET", "NEWSECRET")
	for _, f := range f[:len(f)-1] {
		if f.File != nil && *f.File != "nope.txt" { // a missing file is named, as look does
			t.Errorf("a failure names %q", *f.File)
		}
	}
}
