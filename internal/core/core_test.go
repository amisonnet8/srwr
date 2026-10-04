package core

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/tape"
)

type env struct {
	t     *testing.T
	root  string
	c     *Core
	clock *clock
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) Add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newEnv(t *testing.T) *env {
	t.Helper()
	root := t.TempDir()
	// The temporary directory may itself be a link (macOS); work with the real path.
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	e := &env{t: t, root: root, clock: &clock{t: time.Date(2026, 10, 1, 17, 6, 0, 0, time.UTC)}}
	e.c = e.core()
	return e
}

// core returns another Core on the same workspace, as another srwr mcp process would be.
func (e *env) core() *Core {
	ws, err := session.Open(e.root, session.Options{Now: e.clock.Now, Version: "test"})
	if err != nil {
		e.t.Fatal(err)
	}
	return &Core{WS: ws}
}

func (e *env) write(rel, text string) {
	e.t.Helper()
	p := filepath.Join(e.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) read(rel string) string {
	e.t.Helper()
	b, err := os.ReadFile(filepath.Join(e.root, filepath.FromSlash(rel)))
	if err != nil {
		e.t.Fatal(err)
	}
	return string(b)
}

func (e *env) sel(c *Core, rel string, start, end int) *SelectResult {
	e.t.Helper()
	res, err := c.Select(SelectInput{File: rel, StartLine: start, EndLine: end, Why: "見る"})
	if err != nil {
		e.t.Fatalf("select %s %d..%d: %v", rel, start, end, err)
	}
	return res
}

func (e *env) rep(c *Core, token, newText string) *ReplaceResult {
	e.t.Helper()
	res, err := c.Replace(ReplaceInput{Selection: token, NewText: newText, Why: "変える"})
	if err != nil {
		e.t.Fatalf("replace: %v", err)
	}
	return res
}

// events reads the current tape.
func (e *env) events() []tape.Event {
	e.t.Helper()
	active, err := os.ReadFile(filepath.Join(e.root, ".srwr", "active"))
	if err != nil {
		e.t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(e.root, ".srwr", "tapes", tape.FileName(strings.TrimSpace(string(active))))) //nolint:gosec // a path in a temporary directory
	if err != nil {
		e.t.Fatal(err)
	}
	res := tape.Parse(b)
	if res.Skipped != 0 {
		e.t.Fatalf("the tape has %d lines that cannot be read", res.Skipped)
	}
	return res.Events
}

func (e *env) kinds() []string {
	var k []string
	for _, ev := range e.events() {
		if ev.Type != tape.TypeHeader {
			k = append(k, ev.Type)
		}
	}
	return k
}

// checkTape replays the tape and compares every file it knows with the disk.
func (e *env) checkTape() {
	e.t.Helper()
	st := tape.Build(e.events())
	for name, f := range st.Files {
		if f.Deleted {
			continue
		}
		if got := e.read(name); got != f.Text {
			e.t.Errorf("tape replays %s as %q, but the file holds %q", name, f.Text, got)
		}
	}
}

func wantCode(t *testing.T, err *Error, code string) {
	t.Helper()
	_ = wantErr(t, err, code)
}

// wantErr is wantCode for a test that goes on to look at the error.
func wantErr(t *testing.T, err *Error, code string) *Error {
	t.Helper()
	if err == nil || err.Code != code {
		t.Fatalf("error = %v, want code %s", err, code)
	}
	return err
}

// The example of docs/examples/select-replace.md.
func TestSelectThenReplace(t *testing.T) {
	e := newEnv(t)
	e.write("cmd/main.go", "package main\n\nfunc main() {\n\trun()\n}\n")

	s := e.sel(e.c, "cmd/main.go", 3, 5)
	if want := []string{"func main() {", "\trun()", "}"}; !slices.Equal(s.Lines, want) {
		t.Errorf("lines = %q, want %q", s.Lines, want)
	}
	r := e.rep(e.c, s.Selection, "func main() {\n\tsetup()\n\trun()\n}")
	if r.StartLine != 3 || r.EndLine != 6 {
		t.Errorf("range = %d..%d, want 3..6", r.StartLine, r.EndLine)
	}
	if got, want := e.read("cmd/main.go"), "package main\n\nfunc main() {\n\tsetup()\n\trun()\n}\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
	if got, want := e.kinds(), []string{"snapshot", "select", "replace"}; !slices.Equal(got, want) {
		t.Errorf("tape = %v, want %v", got, want)
	}

	// The token the replace returned works for the next edit, without a select.
	e.rep(e.c, r.Selection, "func main() {\n\trun()\n}")
	if got, want := e.read("cmd/main.go"), "package main\n\nfunc main() {\n\trun()\n}\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}

	// The same token again: the range has changed since.
	_, err := e.c.Replace(ReplaceInput{Selection: s.Selection, NewText: "x", Why: "もう一度"})
	stale := wantErr(t, err, CodeSelectionStale)
	if got, ok := stale.Actual.([]string); !ok || len(got) == 0 {
		t.Errorf("selection_stale has no actual: %#v", stale.Actual)
	}
	e.checkTape()

	ev := e.events()
	rep := ev[len(ev)-2] // the last is the failure of the stale token
	if rep.Type != tape.TypeReplace || rep.Source != tape.SourceMCP || rep.FileShaBefore == rep.FileShaAfter {
		t.Errorf("last event = %+v", rep)
	}
}

func TestTapeContentOfReplace(t *testing.T) {
	e := newEnv(t)
	e.write("a.go", "L1\nL2\nL3\nL4\n")
	s := e.sel(e.c, "a.go", 2, 3)
	r := e.rep(e.c, s.Selection, "X\nY\nZ")
	ev := e.events()
	got := ev[len(ev)-1]
	if got.StartLine != 2 || got.EndLine != 3 || got.OldText != "L2\nL3" || got.NewText != "X\nY\nZ" ||
		got.NewStartLine != 2 || got.NewEndLine != 4 || got.From == nil || *got.From != s.Selection ||
		got.Selection == nil || *got.Selection != r.Selection || got.Why == nil || *got.Why != "変える" {
		t.Errorf("replace event = %+v", got)
	}
	if got.FileShaBefore != tape.Sha("L1\nL2\nL3\nL4\n") || got.FileShaAfter != tape.Sha("L1\nX\nY\nZ\nL4\n") {
		t.Errorf("shas = %s %s", got.FileShaBefore, got.FileShaAfter)
	}
	// seq is the one the token carries.
	if ev[len(ev)-2].Type != tape.TypeSelect || ev[len(ev)-2].Seq != 2 || got.Seq != 3 {
		t.Errorf("seqs = %d %d", ev[len(ev)-2].Seq, got.Seq)
	}
}

func TestInsertDeleteAndBlankLines(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		start, end   int
		newText      string
		want         string
		wantS, wantE int
	}{
		{"insert before a line", "a\nb\n", 2, 1, "x", "a\nx\nb\n", 2, 2},
		{"append at the end", "a\nb\n", 3, 2, "c", "a\nb\nc\n", 3, 3},
		{"append to a file without final newline", "a\nb", 3, 2, "c", "a\nb\nc", 3, 3},
		{"delete a line", "a\nb\nc\n", 2, 2, "", "a\nc\n", 2, 1},
		{"delete all", "a\nb\n", 1, 2, "", "", 1, 0},
		{"one blank line", "a\nb\n", 2, 2, "\n", "a\n\n", 2, 2},
		{"two lines, the last blank", "a\nb\n", 2, 2, "x\n\n", "a\nx\n\n", 2, 3},
		{"newText ending with a newline", "a\nb\n", 2, 2, "x\n", "a\nx\n", 2, 2},
		{"replace the last line of a file without final newline", "a\nb", 2, 2, "B", "a\nB", 2, 2},
		{"write into an empty file", "", 1, 0, "first", "first\n", 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			e.write("f.txt", tt.text)
			s := e.sel(e.c, "f.txt", tt.start, tt.end)
			r := e.rep(e.c, s.Selection, tt.newText)
			if got := e.read("f.txt"); got != tt.want {
				t.Errorf("file = %q, want %q", got, tt.want)
			}
			if r.StartLine != tt.wantS || r.EndLine != tt.wantE {
				t.Errorf("range = %d..%d, want %d..%d", r.StartLine, r.EndLine, tt.wantS, tt.wantE)
			}
			e.checkTape()
			// The returned token is good for another replace of the new range.
			e.rep(e.c, r.Selection, tt.newText)
			e.checkTape()
		})
	}
}

func TestEdgesAreCorrected(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "1\n2\n3\n4\n5\n6\n7\n8\n")
	top := e.sel(e.c, "f.txt", 2, 3)
	bottom := e.sel(e.c, "f.txt", 6, 7)
	middle := e.sel(e.c, "f.txt", 4, 5)

	// Two lines become four above the others: they all move down by two.
	e.rep(e.c, top.Selection, "a\nb\nc\nd")
	e.rep(e.c, bottom.Selection, "X")
	if got, want := e.read("f.txt"), "1\na\nb\nc\nd\n4\n5\nX\n8\n"; got != want {
		t.Fatalf("file = %q, want %q", got, want)
	}
	// The middle one was between the two: it is still found.
	e.rep(e.c, middle.Selection, "M")
	if got, want := e.read("f.txt"), "1\na\nb\nc\nd\nM\nX\n8\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
	e.checkTape()
}

func TestOverlapIsStale(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "1\n2\n3\n4\n5\n")
	a := e.sel(e.c, "f.txt", 2, 4)
	b := e.sel(e.c, "f.txt", 3, 3)
	e.rep(e.c, b.Selection, "three")
	_, err := e.c.Replace(ReplaceInput{Selection: a.Selection, NewText: "x", Why: "w"})
	wantCode(t, err, CodeSelectionStale)
	if got, want := e.read("f.txt"), "1\n2\nthree\n4\n5\n"; got != want {
		t.Errorf("a refused replace changed the file: %q", got)
	}
}

func TestExternalChangeBeforeSelect(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "1\n2\n3\n")
	s := e.sel(e.c, "f.txt", 1, 1)
	e.rep(e.c, s.Selection, "one")
	e.write("f.txt", "zero\none\n2\n3\n") // not by srwr

	s2 := e.sel(e.c, "f.txt", 1, 2)
	if want := []string{"zero", "one"}; !slices.Equal(s2.Lines, want) {
		t.Errorf("lines = %q, want %q", s2.Lines, want)
	}
	if got, want := e.kinds(), []string{"snapshot", "select", "replace", "external", "select"}; !slices.Equal(got, want) {
		t.Fatalf("tape = %v, want %v", got, want)
	}
	ev := e.events()
	x := ev[4]
	wantHunks := []tape.Hunk{{StartLine: 1, EndLine: 0, NewText: "zero", NewStartLine: 1, NewEndLine: 1}}
	if x.Text != nil || !reflect.DeepEqual(x.Hunks, wantHunks) || x.DetectedBy != "select" || x.Author == nil || x.Author.Kind != "external" ||
		x.ExpectedSha != tape.Sha("one\n2\n3\n") || x.ActualSha != tape.Sha("zero\none\n2\n3\n") {
		t.Errorf("external = %+v", x)
	}
	// Seqs have no gaps, and the select's token carries the seq the select got.
	for i, ev := range ev[1:] {
		if ev.Seq != i+1 {
			t.Errorf("event %d has seq %d", i, ev.Seq)
		}
	}
	e.checkTape()
}

func TestExternalChangeBeforeReplace(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "1\n2\n3\n4\n")
	s := e.sel(e.c, "f.txt", 2, 3)

	// A line is added above: the line numbers of the token are now wrong, and srwr cannot tell.
	e.write("f.txt", "0\n1\n2\n3\n4\n")
	_, err := e.c.Replace(ReplaceInput{Selection: s.Selection, NewText: "x", Why: "w"})
	m := wantErr(t, err, CodeSelectionMismatch)
	if got, ok := m.Actual.([]string); !ok || !slices.Equal(got, []string{"1", "2"}) {
		t.Errorf("actual = %#v, want the lines now at 2..3", m.Actual)
	}
	if got, want := e.kinds(), []string{"snapshot", "select", "external", "failure"}; !slices.Equal(got, want) {
		t.Errorf("tape = %v, want %v", got, want)
	}
	if got := e.read("f.txt"); got != "0\n1\n2\n3\n4\n" {
		t.Errorf("a refused replace changed the file: %q", got)
	}

	// The same size of change in place: caught by the content, too.
	s = e.sel(e.c, "f.txt", 2, 3)
	e.write("f.txt", "0\nONE\nTWO\n3\n4\n")
	_, err = e.c.Replace(ReplaceInput{Selection: s.Selection, NewText: "x", Why: "w"})
	wantCode(t, err, CodeSelectionMismatch)
}

func TestSelectAfterExternalWorksAgain(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "1\n2\n")
	e.sel(e.c, "f.txt", 1, 1)
	e.write("f.txt", "1\n2\n3\n")
	s := e.sel(e.c, "f.txt", 3, 3)
	e.rep(e.c, s.Selection, "three")
	if got := e.read("f.txt"); got != "1\n2\nthree\n" {
		t.Errorf("file = %q", got)
	}
	e.checkTape()
}

func TestDeletedFile(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "1\n2\n")
	s := e.sel(e.c, "f.txt", 1, 1)
	if err := os.Remove(filepath.Join(e.root, "f.txt")); err != nil {
		t.Fatal(err)
	}

	_, err := e.c.Select(SelectInput{File: "f.txt", StartLine: 1, EndLine: 1, Why: "w"})
	wantCode(t, err, CodeFileNotFound)
	if got, want := e.kinds(), []string{"snapshot", "select", "external", "failure"}; !slices.Equal(got, want) {
		t.Fatalf("tape = %v, want %v", got, want)
	}
	if x := e.events()[3]; !x.Deleted || x.Text != nil {
		t.Errorf("external = %+v, want a deletion", x)
	}
	// Asking again does not record the deletion twice.
	_, err = e.c.Replace(ReplaceInput{Selection: s.Selection, NewText: "x", Why: "w"})
	wantCode(t, err, CodeFileNotFound)
	if got := len(e.kinds()); got != 5 { // the deletion is recorded once; each refused call is a failure
		t.Errorf("tape has %d events, want 5", got)
	}

	// The file comes back: a snapshot, and the old token no longer fits.
	e.write("f.txt", "new 1\nnew 2\n")
	_, err = e.c.Replace(ReplaceInput{Selection: s.Selection, NewText: "x", Why: "w"})
	wantCode(t, err, CodeSelectionMismatch)
	if got, want := e.kinds(), []string{"snapshot", "select", "external", "failure", "failure", "snapshot", "failure"}; !slices.Equal(got, want) {
		t.Errorf("tape = %v, want %v", got, want)
	}
	e.checkTape()
}

func TestFirstTouchIsASnapshot(t *testing.T) {
	e := newEnv(t)
	e.write("a.go", "a\n")
	e.write("b.go", "b\n")
	e.sel(e.c, "a.go", 1, 1)
	e.sel(e.c, "b.go", 1, 1)
	if got, want := e.kinds(), []string{"snapshot", "select", "snapshot", "select"}; !slices.Equal(got, want) {
		t.Errorf("tape = %v, want %v", got, want)
	}
	if s := e.events()[1]; s.FileHash != tape.FileHash("a.go") || s.Sha != tape.Sha("a\n") {
		t.Errorf("snapshot = %+v", s)
	}
}

func TestTokenOfAnotherSession(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "1\n2\n")
	s := e.sel(e.c, "f.txt", 1, 1)

	e.clock.Add(31 * time.Minute)
	_, err := e.c.Replace(ReplaceInput{Selection: s.Selection, NewText: "x", Why: "w"})
	wantCode(t, err, CodeInvalidSelection)
	if got := e.read("f.txt"); got != "1\n2\n" {
		t.Errorf("file = %q", got)
	}

	// In the new session everything starts again, with a snapshot of the file as it is.
	s2 := e.sel(e.c, "f.txt", 1, 1)
	e.rep(e.c, s2.Selection, "one")
	if got, want := e.kinds(), []string{"failure", "snapshot", "select", "replace"}; !slices.Equal(got, want) { // the refused replace started the new tape
		t.Errorf("the new tape = %v, want %v", got, want)
	}
	e.checkTape()
}

// Another process stands for another srwr mcp the AI's client started.
func TestTokenWorksInAnotherProcess(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "1\n2\n3\n")
	other := e.core()

	s := e.sel(e.c, "f.txt", 2, 2)
	e.rep(other, s.Selection, "two")
	s2 := e.sel(other, "f.txt", 3, 3)
	e.rep(e.c, s2.Selection, "three")
	if got := e.read("f.txt"); got != "1\ntwo\nthree\n" {
		t.Errorf("file = %q", got)
	}
	e.checkTape()
	for i, ev := range e.events()[1:] {
		if ev.Seq != i+1 {
			t.Errorf("event %d has seq %d", i, ev.Seq)
		}
	}
}

func TestTokenOfAnotherWorkspaceOrKey(t *testing.T) {
	a, b := newEnv(t), newEnv(t)
	a.write("f.txt", "1\n")
	b.write("f.txt", "1\n")
	s := a.sel(a.c, "f.txt", 1, 1)
	b.sel(b.c, "f.txt", 1, 1)
	_, err := b.c.Replace(ReplaceInput{Selection: s.Selection, NewText: "x", Why: "w"})
	wantCode(t, err, CodeInvalidSelection)
}

func TestInvalidSelection(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "1\n")
	s := e.sel(e.c, "f.txt", 1, 1)
	for name, tok := range map[string]string{
		"garbage":   "hello",
		"truncated": s.Selection[:len(s.Selection)-2],
		"altered":   s.Selection[:6] + "0" + s.Selection[7:],
		"no prefix": s.Selection[4:],
	} {
		if tok == s.Selection {
			continue
		}
		t.Run(name, func(t *testing.T) {
			_, err := e.c.Replace(ReplaceInput{Selection: tok, NewText: "x", Why: "w"})
			wantCode(t, err, CodeInvalidSelection)
		})
	}
	if e.read("f.txt") != "1\n" {
		t.Error("the file changed")
	}
}

func TestSelectErrors(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "1\n2\n3\n")
	if err := os.MkdirAll(filepath.Join(e.root, "dir"), 0o750); err != nil {
		t.Fatal(err)
	}
	e.write("crlf.txt", "a\r\nb\r\n")
	e.write("nul.bin", "a\x00b\n")
	e.write("latin1.txt", "caf\xe9\n")
	e.write("lone-cr.txt", "a\rb\n")

	tests := []struct {
		name string
		in   SelectInput
		code string
	}{
		{"start 0", SelectInput{File: "f.txt", StartLine: 0, EndLine: 1, Why: "w"}, CodeInvalidRange},
		{"start past the end+1", SelectInput{File: "f.txt", StartLine: 5, EndLine: 5, Why: "w"}, CodeInvalidRange},
		{"end past the end", SelectInput{File: "f.txt", StartLine: 1, EndLine: 4, Why: "w"}, CodeInvalidRange},
		{"end before start-1", SelectInput{File: "f.txt", StartLine: 3, EndLine: 1, Why: "w"}, CodeInvalidRange},
		{"negative", SelectInput{File: "f.txt", StartLine: -1, EndLine: 1, Why: "w"}, CodeInvalidRange},
		{"parent directory", SelectInput{File: "../f.txt", StartLine: 1, EndLine: 1, Why: "w"}, CodeInvalidRange},
		{"hidden parent", SelectInput{File: "dir/../../f.txt", StartLine: 1, EndLine: 1, Why: "w"}, CodeInvalidRange},
		{"absolute", SelectInput{File: filepath.Join(e.root, "f.txt"), StartLine: 1, EndLine: 1, Why: "w"}, CodeInvalidRange},
		{"the directory itself", SelectInput{File: ".", StartLine: 1, EndLine: 1, Why: "w"}, CodeInvalidRange},
		{"drive", SelectInput{File: "C:/x", StartLine: 1, EndLine: 1, Why: "w"}, CodeInvalidRange},
		{"missing", SelectInput{File: "nope.txt", StartLine: 1, EndLine: 1, Why: "w"}, CodeFileNotFound},
		{"a directory", SelectInput{File: "dir", StartLine: 1, EndLine: 1, Why: "w"}, CodeFileNotFound},
		{"CRLF", SelectInput{File: "crlf.txt", StartLine: 1, EndLine: 1, Why: "w"}, CodeUnsupportedFile},
		{"NUL", SelectInput{File: "nul.bin", StartLine: 1, EndLine: 1, Why: "w"}, CodeUnsupportedFile},
		{"not UTF-8", SelectInput{File: "latin1.txt", StartLine: 1, EndLine: 1, Why: "w"}, CodeUnsupportedFile},
		{"a lone CR", SelectInput{File: "lone-cr.txt", StartLine: 1, EndLine: 1, Why: "w"}, CodeUnsupportedFile},
		{"empty why", SelectInput{File: "f.txt", StartLine: 1, EndLine: 1, Why: ""}, CodeInvalidInput},
		{"blank why", SelectInput{File: "f.txt", StartLine: 1, EndLine: 1, Why: " \t\n"}, CodeInvalidInput},
		{"empty file", SelectInput{File: "", StartLine: 1, EndLine: 1, Why: "w"}, CodeInvalidInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := e.c.Select(tt.in)
			wantCode(t, err, tt.code)
			if res != nil {
				t.Errorf("result = %+v with an error", res)
			}
		})
	}

	// invalid_range tells how many lines the file has.
	_, err := e.c.Select(SelectInput{File: "f.txt", StartLine: 9, EndLine: 9, Why: "w"})
	if got, ok := err.Actual.(map[string]int); !ok || got["lineCount"] != 3 {
		t.Errorf("actual = %#v, want lineCount 3", err.Actual)
	}
	if want := "f.txt has 3 lines; startLine=9 endLine=9 is out of range"; err.Message != want {
		t.Errorf("message = %q, want %q", err.Message, want)
	}
	// A refused call leaves only a failure on the tape, besides what was noticed about the file itself.
	kinds := e.kinds()
	if kinds[0] != tape.TypeSnapshot || len(kinds) != len(tests)+2 {
		t.Errorf("tape = %v, want the snapshot made while checking the range and a failure for each of the %d calls", kinds, len(tests)+1)
	}
	for _, k := range kinds[1:] {
		if k != tape.TypeFailure {
			t.Errorf("tape = %v, want only failures after the snapshot", kinds)
			break
		}
	}
}

func TestReplaceInputErrors(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "1\n")
	s := e.sel(e.c, "f.txt", 1, 1)
	tests := []struct {
		name string
		in   ReplaceInput
		code string
	}{
		{"empty selection", ReplaceInput{Selection: "", NewText: "x", Why: "w"}, CodeInvalidInput},
		{"blank why", ReplaceInput{Selection: s.Selection, NewText: "x", Why: "  "}, CodeInvalidInput},
		{"CR in newText", ReplaceInput{Selection: s.Selection, NewText: "x\r\ny", Why: "w"}, CodeInvalidInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := e.c.Replace(tt.in)
			wantCode(t, err, tt.code)
		})
	}
	if e.read("f.txt") != "1\n" {
		t.Error("the file changed")
	}
}

func TestSymlinkOutsideTheWorkspace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on Windows")
	}
	e := newEnv(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(e.root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(e.root, "linkdir")); err != nil {
		t.Fatal(err)
	}
	// A link to the parent of the workspace itself.
	if err := os.Symlink(filepath.Dir(e.root), filepath.Join(e.root, "up")); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"link.txt", "linkdir/secret.txt", "linkdir", "up"} {
		_, err := e.c.Select(SelectInput{File: rel, StartLine: 1, EndLine: 1, Why: "w"})
		wantCode(t, err, CodeInvalidRange)
	}

	// A link to a file inside the workspace is followed, and the link is kept.
	e.write("real.txt", "1\n2\n")
	if err := os.Symlink("real.txt", filepath.Join(e.root, "inside.txt")); err != nil {
		t.Fatal(err)
	}
	s := e.sel(e.c, "inside.txt", 1, 1)
	e.rep(e.c, s.Selection, "one")
	if got := e.read("real.txt"); got != "one\n2\n" {
		t.Errorf("real.txt = %q", got)
	}
	if info, err := os.Lstat(filepath.Join(e.root, "inside.txt")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link was replaced by a file: %v %v", info, err)
	}
}

func TestPermissionsAreKeptAndNoTemporaryFileIsLeft(t *testing.T) {
	e := newEnv(t)
	e.write("run.sh", "1\n2\n")
	if runtime.GOOS != "windows" {
		if err := os.Chmod(filepath.Join(e.root, "run.sh"), 0o750); err != nil { //nolint:gosec // the test checks that this mode is kept
			t.Fatal(err)
		}
	}
	s := e.sel(e.c, "run.sh", 1, 1)
	e.rep(e.c, s.Selection, "one")
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(e.root, "run.sh"))
		if err != nil || info.Mode().Perm() != 0o750 {
			t.Errorf("mode = %v %v, want 0750", info.Mode(), err)
		}
	}
	entries, _ := os.ReadDir(e.root)
	var names []string
	for _, en := range entries {
		names = append(names, en.Name())
	}
	slices.Sort(names)
	if want := []string{".srwr", "run.sh"}; !slices.Equal(names, want) {
		t.Errorf("workspace holds %v, want %v", names, want)
	}
}

func TestPathSpellings(t *testing.T) {
	e := newEnv(t)
	e.write("a/b.txt", "1\n")
	for _, p := range []string{"a/b.txt", "./a/b.txt", "a//b.txt", "a/./b.txt", "a/../a/b.txt"} {
		s := e.sel(e.c, p, 1, 1)
		if len(s.Lines) != 1 {
			t.Errorf("%s: %v", p, s.Lines)
		}
	}
	for _, ev := range e.events() {
		if ev.Type != tape.TypeHeader && ev.File != "a/b.txt" {
			t.Errorf("event for %q: the tape should have the cleaned path", ev.File)
		}
	}
}

func TestRecoversFromAnUnfinishedTape(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "1\n2\n")
	s := e.sel(e.c, "f.txt", 1, 1)
	// The file was written, but the process died before the tape: the next call sees an external change.
	e.write("f.txt", "one\n2\n")
	s2 := e.sel(e.c, "f.txt", 1, 1)
	if s2.Lines[0] != "one" {
		t.Errorf("lines = %q", s2.Lines)
	}
	_, err := e.c.Replace(ReplaceInput{Selection: s.Selection, NewText: "x", Why: "w"})
	wantCode(t, err, CodeSelectionMismatch)
}

// Random edits, checked against a model of the file after every step: the file and the tape agree.
func TestRandomEdits(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // a fixed seed makes the test repeatable
	e := newEnv(t)
	var model []string
	for i := range 20 {
		model = append(model, fmt.Sprintf("line %d", i))
	}
	e.write("f.txt", strings.Join(model, "\n")+"\n")

	for step := range 300 {
		n := len(model)
		start := rng.IntN(n+1) + 1
		end := start - 1 + rng.IntN(min(4, n-start+2)+1)
		end = min(end, n)
		s := e.sel(e.c, "f.txt", start, end)
		if !slices.Equal(s.Lines, model[start-1:end]) {
			t.Fatalf("step %d: lines = %q, want %q", step, s.Lines, model[start-1:end])
		}

		var repl []string
		for range rng.IntN(4) {
			if rng.IntN(4) == 0 {
				repl = append(repl, "")
			} else {
				repl = append(repl, fmt.Sprintf("new %d", rng.IntN(1000)))
			}
		}
		// A client cannot say "no lines" and "one blank line" with the same text, but "\n" is one blank line.
		newText := strings.Join(repl, "\n")
		if len(repl) > 0 {
			newText += "\n"
		}
		r := e.rep(e.c, s.Selection, newText)
		model = slices.Concat(model[:start-1], repl, model[end:])
		if r.StartLine != start || r.EndLine != start+len(repl)-1 {
			t.Fatalf("step %d: range = %d..%d, want %d..%d", step, r.StartLine, r.EndLine, start, start+len(repl)-1)
		}
		want := ""
		if len(model) > 0 {
			want = strings.Join(model, "\n") + "\n"
		}
		if got := e.read("f.txt"); got != want {
			t.Fatalf("step %d: file = %q, want %q", step, got, want)
		}
		if len(model) == 0 {
			model = []string{"again"}
			e.write("f.txt", "again\n")
		}
	}
	e.checkTape()
}

// Several selections made first, then replaced in any order: each lands where its lines are now.
func TestSelectionsReplacedInAnyOrder(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4)) //nolint:gosec // a fixed seed makes the test repeatable
	for round := range 40 {
		e := newEnv(t)
		var lines []string
		for i := range 30 {
			lines = append(lines, fmt.Sprintf("L%d", i))
		}
		e.write("f.txt", strings.Join(lines, "\n")+"\n")

		// Disjoint ranges, in order.
		type part struct {
			start, end int
			tok        string
			repl       []string
		}
		var parts []part
		pos := 1
		for pos <= len(lines) && len(parts) < 6 {
			pos += rng.IntN(3)
			size := rng.IntN(4) // 0 is an insertion before pos
			end := min(pos+size-1, len(lines))
			if pos > len(lines)+1 {
				break
			}
			var repl []string
			for j := range rng.IntN(5) {
				repl = append(repl, fmt.Sprintf("R%d.%d", len(parts), j))
			}
			parts = append(parts, part{start: pos, end: end, repl: repl})
			pos = end + 2 // leave a line between: an insertion right next to a range is fine, but keep them apart
		}
		for i := range parts {
			parts[i].tok = e.sel(e.c, "f.txt", parts[i].start, parts[i].end).Selection
		}

		order := rng.Perm(len(parts))
		for _, i := range order {
			text := ""
			if len(parts[i].repl) > 0 {
				text = strings.Join(parts[i].repl, "\n") + "\n"
			}
			e.rep(e.c, parts[i].tok, text)
		}

		var want []string
		next := 1
		for _, p := range parts {
			want = append(want, lines[next-1:p.start-1]...)
			want = append(want, p.repl...)
			next = p.end + 1
		}
		want = append(want, lines[next-1:]...)
		if got := e.read("f.txt"); got != strings.Join(want, "\n")+"\n" {
			t.Fatalf("round %d (order %v): file =\n%s\nwant\n%s", round, order, got, strings.Join(want, "\n")+"\n")
		}
		e.checkTape()
	}
}

// Many clients at once, each its own process, each on its own file, all in one tape.
func TestConcurrentClients(t *testing.T) {
	e := newEnv(t)
	const clients, rounds = 6, 8
	for i := range clients {
		e.write(fmt.Sprintf("f%d.txt", i), "0\n")
	}
	var wg sync.WaitGroup
	for i := range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := e.core()
			file := fmt.Sprintf("f%d.txt", i)
			for r := 1; r <= rounds; r++ {
				s, err := c.Select(SelectInput{File: file, StartLine: 1, EndLine: 1, Why: "w"})
				if err != nil {
					t.Errorf("client %d: select: %v", i, err)
					return
				}
				if _, err := c.Replace(ReplaceInput{Selection: s.Selection, NewText: fmt.Sprint(r), Why: "w"}); err != nil {
					t.Errorf("client %d: replace: %v", i, err)
					return
				}
			}
		}()
	}
	wg.Wait()
	for i := range clients {
		if got := e.read(fmt.Sprintf("f%d.txt", i)); got != fmt.Sprintf("%d\n", rounds) {
			t.Errorf("f%d.txt = %q", i, got)
		}
	}
	ev := e.events()
	if want := 1 + clients*(1+2*rounds); len(ev) != want {
		t.Errorf("tape has %d events, want %d", len(ev), want)
	}
	for i, x := range ev[1:] {
		if x.Seq != i+1 {
			t.Fatalf("event %d has seq %d", i, x.Seq)
		}
	}
	e.checkTape()
}

func TestExternalChangeIsWrittenAsHunks(t *testing.T) {
	e := newEnv(t)
	var lines []string
	for i := 1; i <= 50; i++ {
		lines = append(lines, strconv.Itoa(i))
	}
	e.write("f.txt", strings.Join(lines, "\n")+"\n")
	e.sel(e.c, "f.txt", 1, 1)
	lines[2] = "three"
	lines = append(lines[:40], append([]string{"x", "y"}, lines[41:]...)...) // line 41 becomes two lines
	changed := strings.Join(lines, "\n") + "\n"
	e.write("f.txt", changed) // not by srwr, two places far apart
	e.sel(e.c, "f.txt", 1, 1)

	x := e.events()[3]
	if x.Type != tape.TypeExternal || x.Text != nil || len(x.Hunks) != 2 {
		t.Fatalf("external = %+v, want two hunks and no text", x)
	}
	if got, want := e.kinds(), []string{"snapshot", "select", "external", "select"}; !slices.Equal(got, want) {
		t.Errorf("tape = %v, want %v", got, want)
	}
	if got := tape.Build(e.events()).Files["f.txt"].Text; got != changed {
		t.Errorf("the tape says %q, want the file", got)
	}
	e.checkTape()
}

func TestExternalChangeOfTheFinalLineBreakWritesTheWholeText(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "a\nb\n")
	e.sel(e.c, "f.txt", 1, 1)
	e.write("f.txt", "a\nb") // only the line break at the end is gone
	e.sel(e.c, "f.txt", 1, 1)
	x := e.events()[3]
	if x.Type != tape.TypeExternal || x.Hunks != nil || x.Text == nil || *x.Text != "a\nb" {
		t.Errorf("external = %+v, want the whole text", x)
	}
	e.checkTape()
}

func TestReplaceReturnsWhatIsThereNow(t *testing.T) {
	tests := []struct {
		name                 string
		file                 string
		start, end           int
		newText              string
		lines, before, after []string
	}{
		{"middle", "1\n2\n3\n4\n5\n6\n7\n", 4, 4, "X", []string{"X"}, []string{"2", "3"}, []string{"5", "6"}},
		{"near the start", "1\n2\n3\n4\n", 2, 2, "X", []string{"X"}, []string{"1"}, []string{"3", "4"}},
		{"at the start", "1\n2\n3\n", 1, 1, "X", []string{"X"}, []string{}, []string{"2", "3"}},
		{"near the end", "1\n2\n3\n4\n", 3, 3, "X", []string{"X"}, []string{"1", "2"}, []string{"4"}},
		{"at the end", "1\n2\n3\n", 3, 3, "X", []string{"X"}, []string{"1", "2"}, []string{}},
		{"the whole file", "1\n2\n", 1, 2, "a\nb\nc", []string{"a", "b", "c"}, []string{}, []string{}},
		{"a file of one line", "1\n", 1, 1, "X", []string{"X"}, []string{}, []string{}},
		{"a deletion keeps the lines around", "1\n2\n3\n4\n5\n", 3, 3, "", []string{}, []string{"1", "2"}, []string{"4", "5"}},
		{"a deletion of everything", "1\n2\n", 1, 2, "", []string{}, []string{}, []string{}},
		{"an insertion", "1\n2\n3\n4\n", 3, 2, "new", []string{"new"}, []string{"1", "2"}, []string{"3", "4"}},
		{"an append to the end", "1\n2\n", 3, 2, "new", []string{"new"}, []string{"1", "2"}, []string{}},
		{"many lines become one", "1\n2\n3\n4\n5\n6\n", 2, 5, "X", []string{"X"}, []string{"1"}, []string{"6"}},
		{"one line becomes many", "1\n2\n3\n", 2, 2, "a\nb\nc\nd", []string{"a", "b", "c", "d"}, []string{"1"}, []string{"3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			e.write("a.txt", tt.file)
			s := e.sel(e.c, "a.txt", tt.start, tt.end)
			r := e.rep(e.c, s.Selection, tt.newText)
			if !slices.Equal(r.Lines, tt.lines) || r.Lines == nil {
				t.Errorf("lines = %#v, want %#v", r.Lines, tt.lines)
			}
			if !slices.Equal(r.Before, tt.before) || r.Before == nil {
				t.Errorf("before = %#v, want %#v", r.Before, tt.before)
			}
			if !slices.Equal(r.After, tt.after) || r.After == nil {
				t.Errorf("after = %#v, want %#v", r.After, tt.after)
			}
		})
	}
}
