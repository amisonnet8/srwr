package core

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/tape"
)

func (e *env) sub(files []string, old, repl string, count int) (*ReplaceResult, *Error) {
	return e.c.Replace(ReplaceInput{Files: files, Old: old, New: repl, Count: count, Why: "rename the helper"})
}

func (e *env) eventsOf(typ string) []tape.Event {
	var got []tape.Event
	for _, ev := range e.events() {
		if ev.Type == typ {
			got = append(got, ev)
		}
	}
	return got
}

func (e *env) replaces() []tape.Event { return e.eventsOf(tape.TypeReplace) }

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
	// One entry for each place: the lines as they are now, and the line before and the line after.
	wantA := ReplaceFile{File: "a.go", Count: 2, Hits: []ReplaceHit{
		{StartLine: 2, EndLine: 2, Lines: []string{"bar(1)"}, Before: []string{"x"}, After: []string{"y"}},
		{StartLine: 5, EndLine: 5, Lines: []string{"bar(2)"}, Before: []string{"z"}, After: []string{"w"}},
	}}
	if !reflect.DeepEqual(res.Files[0], wantA) {
		t.Errorf("a.go = %+v, want %+v", res.Files[0], wantA)
	}
	if b := res.Files[1]; b.File != "b.go" || b.Count != 1 || len(b.Hits) != 1 || len(b.Hits[0].Before) != 0 || len(b.Hits[0].After) != 0 {
		t.Errorf("b.go = %+v: no line before or after is []", b)
	}
	// One replace for each file that changed, with the same why, the tool and the number of places.
	rs := e.replaces()
	if len(rs) != 2 {
		t.Fatalf("%d replaces", len(rs))
	}
	for i, r := range rs {
		if r.Source != tape.SourceMCP || r.Why == nil || *r.Why != "rename the helper" || r.From != nil || r.Selection != nil {
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
	if len(f) != 1 || f[0].Tool != "replace" || f[0].Code != CodeCountMismatch || f[0].File != nil || f[0].Why == nil {
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
	want := []ReplaceHit{
		{StartLine: 2, EndLine: 2, Lines: []string{"new"}, Before: []string{"1"}, After: []string{"2"}},
		{StartLine: 4, EndLine: 4, Lines: []string{"new"}, Before: []string{"2"}, After: []string{"3"}},
	}
	if !reflect.DeepEqual(res.Files[0].Hits, want) {
		t.Errorf("hits = %+v, want %+v", res.Files[0].Hits, want)
	}
	e.checkTape()

	// A place that spans lines is a range of lines.
	e.write("c.txt", "a\nx\ny\nb\nx\ny\n")
	res, err = e.sub([]string{"c.txt"}, "x\ny", "p\nq\nr", 2)
	if err != nil {
		t.Fatal(err)
	}
	if h := res.Files[0].Hits; len(h) != 2 || h[0].StartLine != 2 || h[0].EndLine != 4 || strings.Join(h[0].Lines, "|") != "p|q|r" || h[1].StartLine != 6 || h[1].EndLine != 8 {
		t.Errorf("hits = %+v", h)
	}

	// Deleting the text leaves the line where it was.
	e.write("b.txt", "a\nX\nb\nX\nc\n")
	res, err = e.sub([]string{"b.txt"}, "X\n", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.read("b.txt"); got != "a\nb\nc\n" || len(res.Files[0].Hits) != 2 || res.Files[0].Hits[0].StartLine != 2 || strings.Join(res.Files[0].Hits[0].Lines, "|") != "b" {
		t.Errorf("file = %q, result = %+v", got, res.Files[0])
	}
	e.checkTape()
}

func TestSubPlacesOnOneLineAreOneEntry(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", "ab ab\nxxx\n")
	res, err := e.sub([]string{"a.txt"}, "ab", "c", 2)
	if err != nil {
		t.Fatal(err)
	}
	if e.read("a.txt") != "c c\nxxx\n" {
		t.Errorf("file = %q", e.read("a.txt"))
	}
	if f := res.Files[0]; f.Count != 2 || len(f.Hits) != 1 || f.Hits[0].StartLine != 1 || f.Hits[0].EndLine != 1 {
		t.Errorf("result = %+v: two places on a line are one entry, and the count is of the places", f)
	}
	e.checkTape()
}

func TestSubListsAtMostTwentyPlacesOfAFile(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", strings.Repeat("foo\nx\n", 25))
	res, err := e.sub([]string{"a.txt"}, "foo", "bar", 25)
	if err != nil {
		t.Fatal(err)
	}
	if f := res.Files[0]; f.Count != 25 || len(f.Hits) != 20 || f.More != 5 {
		t.Errorf("count %d, %d listed, %d more", f.Count, len(f.Hits), f.More)
	}
	if strings.Contains(e.read("a.txt"), "foo") {
		t.Error("not every place was changed")
	}
}

// replace is for 2 or more places. One place is answered with where it is and the edit call to make; nothing is changed.
func TestSubOfOnePlaceIsUseEdit(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", "keep\nxx and xx\nkeep\n")
	e.write("b.txt", "none\n")
	e.write("c.txt", "keep\nfoo SECRETWORD bar\nkeep\n")
	_, err := e.sub([]string{"b.txt", "c.txt"}, "foo SECRETWORD", "baz", 1)
	wantCode(t, err, CodeUseEdit)
	if strings.Contains(err.Message, "SECRETWORD") || !strings.Contains(err.Message, "c.txt line 2") {
		t.Errorf("message = %q: it names the place and has none of the file", err.Message)
	}
	if e.read("c.txt") != "keep\nfoo SECRETWORD bar\nkeep\n" || len(e.replaces()) != 0 {
		t.Error("something was changed")
	}
	actual, _ := err.Actual.(map[string]any)
	hits, _ := actual["hits"].([]useEditHit)
	call, _ := actual["edit"].(useEditCall)
	if len(hits) != 1 || hits[0].File != "c.txt" || hits[0].StartLine != 2 || strings.Join(hits[0].Lines, "|") != "foo SECRETWORD bar" {
		t.Errorf("hits = %+v", hits)
	}
	if call != (useEditCall{File: "c.txt", StartLine: 2, EndLine: 2, Expect: "foo SECRETWORD bar", NewText: "baz bar"}) {
		t.Errorf("edit = %+v", call)
	}
	// The failure is on the tape without the texts.
	f := e.failures()
	if len(f) != 1 || f[0].Tool != "replace" || f[0].Code != CodeUseEdit || f[0].File != nil {
		t.Errorf("failures = %+v", f)
	}
	for _, ev := range e.eventsOf(tape.TypeFailure) { // the snapshot holds the file, but the failure holds neither text
		if b, _ := json.Marshal(ev); strings.Contains(string(b), "SECRETWORD") || strings.Contains(string(b), "baz") {
			t.Errorf("the failure holds a text: %s", b)
		}
	}

	// The call it gives is the one that works.
	if _, err := e.c.Edit(EditInput{File: call.File, HasLines: true, StartLine: call.StartLine, EndLine: call.EndLine, Expect: &call.Expect, NewText: call.NewText, Why: "change it"}); err != nil {
		t.Fatal(err)
	}
	if e.read("c.txt") != "keep\nbaz bar\nkeep\n" {
		t.Errorf("file = %q", e.read("c.txt"))
	}

	// Places that do not overlap are counted as replace counts them: "xx" is one place of "xxx".
	e.write("d.txt", "xxx\n")
	_, err = e.sub([]string{"d.txt"}, "xx", "y", 1)
	wantCode(t, err, CodeUseEdit)

	// With a count of 1 and no place, or more than one, it is the count that is wrong.
	_, err = e.sub([]string{"b.txt"}, "zzz", "y", 1)
	wantCode(t, err, CodeCountMismatch)
	_, err = e.sub([]string{"a.txt"}, "xx", "y", 1)
	wantCode(t, err, CodeCountMismatch)
	// And a count of 2 for one place is a count that is wrong, not a case for edit.
	_, err = e.sub([]string{"d.txt"}, "xx", "y", 2)
	wantCode(t, err, CodeCountMismatch)
}

// A change that cannot be told in lines has no edit to give: the places are still told.
func TestSubOfOnePlaceThatEditCannotTell(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", "x\nfoo\n")
	_, err := e.sub([]string{"a.txt"}, "foo\n", "foo", 1)
	wantCode(t, err, CodeUseEdit)
	actual, _ := err.Actual.(map[string]any)
	if _, has := actual["edit"]; has || actual["hits"] == nil || strings.Contains(err.Message, "actual.edit") {
		t.Errorf("error = %+v", err)
	}
}

func TestSubInputIsChecked(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", "foo\n")
	for name, in := range map[string]ReplaceInput{
		"no files":   {Old: "foo", New: "x", Count: 1, Why: "w"},
		"empty old":  {Files: []string{"a.txt"}, New: "x", Count: 1, Why: "w"},
		"zero count": {Files: []string{"a.txt"}, Old: "foo", New: "x", Why: "w"},
		"negative":   {Files: []string{"a.txt"}, Old: "foo", New: "x", Count: -1, Why: "w"},
		"blank why":  {Files: []string{"a.txt"}, Old: "foo", New: "x", Count: 1, Why: " "},
		"CR in new":  {Files: []string{"a.txt"}, Old: "foo", New: "x\r\n", Count: 1, Why: "w"},
		"twice":      {Files: []string{"a.txt", "./a.txt"}, Old: "foo", New: "x", Count: 2, Why: "w"},
		"empty file": {Files: []string{""}, Old: "foo", New: "x", Count: 1, Why: "w"},
	} {
		_, err := e.c.Replace(in)
		wantCode(t, err, CodeInvalidInput)
		if name != "" && e.read("a.txt") != "foo\n" {
			t.Errorf("%s: the file was changed", name)
		}
	}
	e.checkTape()
}

func TestSubThatAddsOrRemovesTheFinalLineBreakIsInvalid(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", "foo\nbar foo\n")
	_, err := e.sub([]string{"a.txt"}, "foo\n", "foo", 2)
	wantCode(t, err, CodeInvalidInput)
	if e.read("a.txt") != "foo\nbar foo\n" {
		t.Error("the file was changed")
	}
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
