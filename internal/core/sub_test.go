package core

import (
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/tape"
)

func (e *env) sub(files []string, old, repl string, count int) (*SubResult, *Error) {
	return e.c.Sub(SubInput{Files: files, Old: old, New: repl, Count: count, Why: "rename the helper"})
}

func (e *env) replaces() []tape.Event {
	var got []tape.Event
	for _, ev := range e.events() {
		if ev.Type == tape.TypeReplace {
			got = append(got, ev)
		}
	}
	return got
}

func TestSubChangesEveryPlaceInEveryFile(t *testing.T) {
	e := newEnv(t)
	e.write("a.go", "x\nfoo(1)\ny\nz\nfoo(2)\nw\n")
	e.write("b.go", "foo(3)\n")
	e.write("c.go", "nothing here\n")
	res, err := e.sub([]string{"a.go", "b.go", "c.go"}, "foo(", "bar(", 3)
	if err != nil {
		t.Fatal(err)
	}
	if e.read("a.go") != "x\nbar(1)\ny\nz\nbar(2)\nw\n" || e.read("b.go") != "bar(3)\n" || e.read("c.go") != "nothing here\n" {
		t.Errorf("files = %q %q %q", e.read("a.go"), e.read("b.go"), e.read("c.go"))
	}
	if res.Count != 3 || len(res.Files) != 2 {
		t.Fatalf("result = %+v", res)
	}
	a := res.Files[0]
	if a.File != "a.go" || a.Hits != 2 || a.StartLine != 2 || a.EndLine != 5 || a.Selection == "" ||
		strings.Join(a.Lines, "|") != "bar(1)|y|z|bar(2)" {
		t.Errorf("a.go = %+v", a)
	}
	// One replace for each file that changed, with the same why, the tool and the number of places.
	rs := e.replaces()
	if len(rs) != 2 {
		t.Fatalf("%d replaces", len(rs))
	}
	for i, r := range rs {
		if r.HookTool != "sub" || r.Source != tape.SourceMCP || r.Why == nil || *r.Why != "rename the helper" || r.From != nil {
			t.Errorf("replace %d = %+v", i, r)
		}
	}
	if rs[0].Hits != 2 || rs[1].Hits != 1 || rs[0].StartLine != 2 || rs[0].EndLine != 5 || rs[0].OldText != "foo(1)\ny\nz\nfoo(2)" {
		t.Errorf("replaces = %+v %+v", rs[0], rs[1])
	}
	e.checkTape()
}

func TestSubChangesNothingWhenTheCountIsWrong(t *testing.T) {
	e := newEnv(t)
	e.write("a.go", "foo\nfoo\n")
	e.write("b.go", "foo\n")
	_, err := e.sub([]string{"a.go", "b.go"}, "foo", "bar", 4)
	wantCode(t, err, CodeCountMismatch)
	if !strings.Contains(err.Message, "expected 4") || !strings.Contains(err.Message, "found 3") || !strings.Contains(err.Message, "a.go: 2, b.go: 1") {
		t.Errorf("message = %q", err.Message)
	}
	if e.read("a.go") != "foo\nfoo\n" || e.read("b.go") != "foo\n" || len(e.replaces()) != 0 {
		t.Error("something was changed")
	}
	f := e.failures()
	if len(f) != 1 || f[0].Tool != "sub" || f[0].Code != CodeCountMismatch || f[0].File != nil || f[0].Why == nil {
		t.Errorf("failures = %+v", f)
	}
	e.noSecretOnTape("bar")
}

func TestSubFailureOfOneFileNamesIt(t *testing.T) {
	e := newEnv(t)
	e.write("a.go", "foo\n")
	_, err := e.sub([]string{"a.go"}, "foo", "bar", 2)
	wantCode(t, err, CodeCountMismatch)
	if f := e.failures()[0]; f.File == nil || *f.File != "a.go" {
		t.Errorf("failure = %+v", f)
	}
}

func TestSubMultiLineTexts(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", "1\nold\ntext\n2\nold\ntext\n3\n")
	res, err := e.sub([]string{"a.txt"}, "old\ntext\n", "new\n", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.read("a.txt"); got != "1\nnew\n2\nnew\n3\n" {
		t.Errorf("file = %q", got)
	}
	if f := res.Files[0]; f.StartLine != 2 || f.EndLine != 4 || strings.Join(f.Lines, "|") != "new|2|new" {
		t.Errorf("result = %+v", f)
	}
	e.checkTape()

	// Deleting everything that is in the range leaves an empty range.
	e.write("b.txt", "a\nX\nb\n")
	res, err = e.sub([]string{"b.txt"}, "X\n", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.read("b.txt"); got != "a\nb\n" || len(res.Files[0].Lines) != 0 || res.Files[0].EndLine != 1 {
		t.Errorf("file = %q, result = %+v", got, res.Files[0])
	}
	e.checkTape()
}

func TestSubTwoPlacesOnOneLineAndOverlaps(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", "ab ab\nxxx\n")
	if _, err := e.sub([]string{"a.txt"}, "ab", "c", 2); err != nil {
		t.Fatal(err)
	}
	if e.read("a.txt") != "c c\nxxx\n" {
		t.Errorf("file = %q", e.read("a.txt"))
	}
	// "xx" in "xxx" is one place, since places do not overlap.
	if _, err := e.sub([]string{"a.txt"}, "xx", "y", 1); err != nil {
		t.Fatal(err)
	}
	if e.read("a.txt") != "c c\nyx\n" {
		t.Errorf("file = %q", e.read("a.txt"))
	}
	e.checkTape()
}

func TestSubInputIsChecked(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", "foo\n")
	for name, in := range map[string]SubInput{
		"no files":     {Old: "foo", New: "x", Count: 1, Why: "w"},
		"empty old":    {Files: []string{"a.txt"}, New: "x", Count: 1, Why: "w"},
		"zero count":   {Files: []string{"a.txt"}, Old: "foo", New: "x", Why: "w"},
		"blank why":    {Files: []string{"a.txt"}, Old: "foo", New: "x", Count: 1, Why: " "},
		"CR in new":    {Files: []string{"a.txt"}, Old: "foo", New: "x\r\n", Count: 1, Why: "w"},
		"twice":        {Files: []string{"a.txt", "./a.txt"}, Old: "foo", New: "x", Count: 2, Why: "w"},
		"empty file":   {Files: []string{""}, Old: "foo", New: "x", Count: 1, Why: "w"},
		"no final \\n": {Files: []string{"a.txt"}, Old: "foo\n", New: "foo", Count: 1, Why: "w"},
	} {
		_, err := e.c.Sub(in)
		wantCode(t, err, CodeInvalidInput)
		if name != "" && e.read("a.txt") != "foo\n" {
			t.Errorf("%s: the file was changed", name)
		}
	}
	e.checkTape()
}

func TestSubPathsAndFilesThatCannotBeUsed(t *testing.T) {
	e := newEnv(t)
	e.write("a.go", "foo\n")
	e.write(".env", "foo "+secret)
	e.write("crlf.txt", "foo\r\n")
	_, err := e.sub([]string{"a.go", "missing.go"}, "foo", "bar", 1)
	wantCode(t, err, CodeFileNotFound)
	_, err = e.sub([]string{"a.go", ".env"}, "foo", "bar", 2)
	wantCode(t, err, CodeIgnoredFile)
	_, err = e.sub([]string{"a.go", "crlf.txt"}, "foo", "bar", 2)
	wantCode(t, err, CodeUnsupportedFile)
	_, err = e.sub([]string{"a.go", "../outside"}, "foo", "bar", 1)
	wantCode(t, err, CodeInvalidRange)
	if e.read("a.go") != "foo\n" {
		t.Error("a.go was changed although another file could not be used")
	}
	e.noSecretOnTape(".env", "../outside")
	for _, f := range e.failures() {
		if f.File != nil {
			t.Errorf("failure about several files names %q", *f.File)
		}
	}
}

func TestSubAfterAnEditOutsideSrwr(t *testing.T) {
	e := newEnv(t)
	e.write("a.go", "foo\n")
	s := e.sel(e.c, "a.go", 1, 1)
	_ = s
	e.write("a.go", "foo\nfoo\n") // changed outside srwr
	if _, err := e.sub([]string{"a.go"}, "foo", "bar", 2); err != nil {
		t.Fatal(err)
	}
	if got := e.kinds(); !strings.Contains(strings.Join(got, ","), "external,replace") {
		t.Errorf("kinds = %v", got)
	}
	e.checkTape()
}

func TestASubTokenCanBeUsedByReplace(t *testing.T) {
	e := newEnv(t)
	e.write("a.go", "x\nfoo\ny\nfoo\nz\n")
	res, err := e.sub([]string{"a.go"}, "foo", "bar", 2)
	if err != nil {
		t.Fatal(err)
	}
	e.rep(e.c, res.Files[0].Selection, "BAR1\ny\nBAR2")
	if e.read("a.go") != "x\nBAR1\ny\nBAR2\nz\n" {
		t.Errorf("file = %q", e.read("a.go"))
	}
	e.checkTape()
}
