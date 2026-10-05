package core

import (
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/tape"
)

// oldNew is an edit of a part of the text by its path.
func (e *env) oldNew(in EditInput) (*EditResult, *Error) {
	in.File, in.Why = "f.txt", "change a part"
	return e.c.Edit(in)
}

func TestEditOldNew(t *testing.T) {
	cases := []struct {
		name       string
		file       string
		in         EditInput
		want       string
		start, end int // the range edited, from the result
		code       string
	}{
		{name: "a part of a line", file: "a\nfoo(bar, baz)\nc\n", in: EditInput{Old: str("bar"), New: str("BAR")},
			want: "a\nfoo(BAR, baz)\nc\n", start: 2, end: 2},
		{name: "several lines", file: "a\nx := 1\ny := 2\nc\n", in: EditInput{Old: str("1\ny"), New: str("1\nz")},
			want: "a\nx := 1\nz := 2\nc\n", start: 2, end: 3},
		{name: "new has a line break: lines grow", file: "a\nfoo(bar)\nc\n", in: EditInput{Old: str("foo("), New: str("foo(\n\t")},
			want: "a\nfoo(\n\tbar)\nc\n", start: 2, end: 2},
		{name: "new is empty: a part is deleted", file: "a\nfoo bar\nc\n", in: EditInput{Old: str(" bar"), New: str("")},
			want: "a\nfoo\nc\n", start: 2, end: 2},
		{name: "old ends with a line break: the line goes", file: "a\nb\nc\n", in: EditInput{Old: str("b\n"), New: str("")},
			want: "a\nc\n", start: 2, end: 2},
		{name: "old ends with a line break, new does not: the lines join", file: "a\nb\nc\n", in: EditInput{Old: str("b\n"), New: str("B")},
			want: "a\nBc\n", start: 2, end: 3},
		{name: "the first line", file: "foo\nb\n", in: EditInput{Old: str("foo"), New: str("x")}, want: "x\nb\n", start: 1, end: 1},
		{name: "the last line", file: "a\nfoo\n", in: EditInput{Old: str("foo"), New: str("x")}, want: "a\nx\n", start: 2, end: 2},
		{name: "the last line break, joined to nothing", file: "a\nb\n", in: EditInput{Old: str("b\n"), New: str("B")}, want: "a\nB\n", start: 2, end: 2},
		{name: "in two places, line numbers tell", file: "x\nfoo\nx\nfoo\n", in: EditInput{Old: str("foo"), New: str("F"), HasLines: true, StartLine: 4, EndLine: 4},
			want: "x\nfoo\nx\nF\n", start: 4, end: 4},
		{name: "wrong line numbers, one place in the file", file: "x\nfoo\nx\n", in: EditInput{Old: str("foo"), New: str("F"), HasLines: true, StartLine: 3, EndLine: 3},
			want: "x\nF\nx\n", start: 2, end: 2},
		{name: "in two places", file: "x\nfoo\nx\nfoo\n", in: EditInput{Old: str("foo"), New: str("F")}, code: CodeContentAmbiguous},
		{name: "in two places in the lines given", file: "foo foo\nx\n", in: EditInput{Old: str("foo"), New: str("F"), HasLines: true, StartLine: 1, EndLine: 1}, code: CodeContentAmbiguous},
		{name: "places that overlap are two", file: "aaa\n", in: EditInput{Old: str("aa"), New: str("b")}, code: CodeContentAmbiguous},
		{name: "not in the file", file: "a\nb\n", in: EditInput{Old: str("zz"), New: str("y")}, code: CodeContentNotFound},
		{name: "old in two lines, one place only in the file", file: "a\nb\nc\n", in: EditInput{Old: str("a\nc"), New: str("y")}, code: CodeContentNotFound},
		{name: "line numbers out of range", file: "a\n", in: EditInput{Old: str("a"), New: str("b"), HasLines: true, StartLine: 3, EndLine: 4}, code: CodeInvalidRange},
		{name: "empty old", file: "a\n", in: EditInput{Old: str(""), New: str("b")}, code: CodeInvalidInput},
		{name: "old is new", file: "a\n", in: EditInput{Old: str("a"), New: str("a")}, code: CodeInvalidInput},
		{name: "CR in old", file: "a\n", in: EditInput{Old: str("a\r"), New: str("b")}, code: CodeInvalidInput},
		{name: "CR in new", file: "a\n", in: EditInput{Old: str("a"), New: str("b\r")}, code: CodeInvalidInput},
		{name: "old only", file: "a\n", in: EditInput{Old: str("a")}, code: CodeInvalidInput},
		{name: "with expect", file: "a\n", in: EditInput{Old: str("a"), New: str("b"), Expect: str("a")}, code: CodeInvalidInput},
		{name: "with insert", file: "a\n", in: EditInput{Old: str("a"), New: str("b"), Insert: "after"}, code: CodeInvalidInput},
		{name: "with newText", file: "a\n", in: EditInput{Old: str("a"), New: str("b"), NewText: "c"}, code: CodeInvalidInput},
		{name: "with an empty range", file: "a\n", in: EditInput{Old: str("a"), New: str("b"), HasLines: true, StartLine: 2, EndLine: 1}, code: CodeInvalidInput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.write("f.txt", tc.file)
			res, err := e.oldNew(tc.in)
			if tc.code != "" {
				wantCode(t, err, tc.code)
				if e.read("f.txt") != tc.file {
					t.Error("the file was changed")
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %+v", err)
			}
			if got := e.read("f.txt"); got != tc.want {
				t.Errorf("file = %q, want %q", got, tc.want)
			}
			ev := e.events()
			last := ev[len(ev)-1]
			if last.Type != tape.TypeEdit || last.From != nil || last.StartLine != last.NewStartLine {
				t.Errorf("tape event = %+v", last)
			}
			if last.StartLine != tc.start {
				t.Errorf("range starts at %d, want %d (end %d)", last.StartLine, tc.start, last.EndLine)
			}
			if last.EndLine != tc.end {
				t.Errorf("range ends at %d, want %d", last.EndLine, tc.end)
			}
			if strings.Contains(res.Selection, " ") || res.Selection == "" {
				t.Errorf("selection = %q", res.Selection)
			}
			e.checkTape()
		})
	}
}

func TestEditOldNewAfterTheFileMoved(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "a\nb\nfoo\nx\nfoo\n")
	e.sel(e.c, "f.txt", 1, 5) // the look that line numbers follow
	s := e.sel(e.c, "f.txt", 1, 1)
	e.rep(e.c, s.Selection, "a\nnew") // line 3 is now line 4
	// The client still says line 3: the corrected range holds the one foo it meant.
	if _, err := e.oldNew(EditInput{Old: str("foo"), New: str("F"), HasLines: true, StartLine: 5, EndLine: 5}); err != nil {
		t.Fatalf("err = %+v", err)
	}
	if got := e.read("f.txt"); got != "a\nnew\nb\nfoo\nx\nF\n" {
		t.Errorf("file = %q", got)
	}
}

func TestEditOldNewTapeHasNoOldOrNew(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "a\nfoo(bar)\n")
	if _, err := e.oldNew(EditInput{Old: str("zzbar"), New: str("secretnew")}); err == nil {
		t.Fatal("want a failure")
	}
	e.noContentInFailures("zzbar", "secretnew")
	fs := e.failures()
	if len(fs) != 1 || fs[0].Code != CodeContentNotFound || fs[0].File == nil || *fs[0].File != "f.txt" {
		t.Errorf("failures = %+v", fs)
	}
}

func TestEditOldNewNearMatches(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "a\n\tfoo(  bar)\n")
	err := e.oldNewErr(EditInput{Old: str("foo( bar)"), New: str("x")})
	if len(err.NearMatches) != 1 || err.NearMatches[0].StartLine != 2 {
		t.Errorf("near = %+v", err.NearMatches)
	}
}

func (e *env) oldNewErr(in EditInput) *Error {
	e.t.Helper()
	_, err := e.oldNew(in)
	return wantErr(e.t, err, CodeContentNotFound)
}

func TestEditOldNewIsNotRecordedForIgnoredFiles(t *testing.T) {
	e := newEnv(t)
	e.write(".env", "KEY=secret\n")
	_, err := e.c.Edit(EditInput{File: ".env", Old: str("secret"), New: str("x"), Why: "w"})
	wantCode(t, err, CodeIgnoredFile)
	e.noSecretOnTape(".env")
}

func TestEditsWithOldNew(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "a := 1\nb := 2\nc := 3\n")
	res, err := e.c.Edits(EditsInput{Why: "w", Edits: []EditInput{
		{File: "f.txt", Old: str("1"), New: str("one")},
		{File: "f.txt", Expect: str("c := 3"), NewText: "c := three"},
		{File: "f.txt", Old: str("b := 2"), New: str("b := 2\nb2 := 2")},
	}})
	if err != nil {
		t.Fatalf("err = %+v", err)
	}
	if got := e.read("f.txt"); got != "a := one\nb := 2\nb2 := 2\nc := three\n" {
		t.Errorf("file = %q", got)
	}
	if len(res.Edits) != 3 {
		t.Fatalf("edits = %d", len(res.Edits))
	}
	e.checkTape()
}
