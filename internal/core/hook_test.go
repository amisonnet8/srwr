package core

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/tape"
)

func (e *env) hook(req HookRequest) []string {
	e.t.Helper()
	notes, err := e.c.Hook(req)
	if err != nil {
		e.t.Fatal(err)
	}
	return notes
}

func TestHookSelectRanges(t *testing.T) {
	text := "a\nb\nc\nd\ne\n"
	for name, tc := range map[string]struct {
		r          HookRange
		start, end int
	}{
		"all":            {HookRange{Mode: RangeAll}, 1, 5},
		"lines":          {HookRange{Mode: RangeLines, A: 2, B: 4}, 2, 4},
		"to the end":     {HookRange{Mode: RangeLines, A: 4}, 4, 5},
		"past the end":   {HookRange{Mode: RangeLines, A: 4, B: 99}, 4, 5},
		"tail":           {HookRange{Mode: RangeTail, A: 2}, 4, 5},
		"tail too many":  {HookRange{Mode: RangeTail, A: 99}, 1, 5},
		"first line":     {HookRange{Mode: RangeLines, A: 1, B: 1}, 1, 1},
		"lines before 1": {HookRange{Mode: RangeLines, A: 0, B: 2}, 1, 2},
	} {
		e := newEnv(t)
		e.write("f.txt", text)
		e.hook(HookRequest{Selects: []HookSelect{{File: "f.txt", Range: tc.r, Tool: "Read"}}})
		evs := e.events()
		var sel tape.Event
		for _, ev := range evs {
			if ev.Type == tape.TypeSelect {
				sel = ev
			}
		}
		if sel.StartLine != tc.start || sel.EndLine != tc.end {
			t.Errorf("%s: %d..%d, want %d..%d", name, sel.StartLine, sel.EndLine, tc.start, tc.end)
		}
		if sel.Source != tape.SourceHook || sel.HookTool != "Read" || sel.Why != nil || sel.Selection != nil {
			t.Errorf("%s: a hook select has source hook, the tool, no why and no token: %+v", name, sel)
		}
		if got := e.kinds(); !reflect.DeepEqual(got, []string{"snapshot", "select"}) {
			t.Errorf("%s: kinds = %v", name, got)
		}
	}
}

func TestHookSelectNotesWhatItLeavesOut(t *testing.T) {
	e := newEnv(t)
	e.write("empty.txt", "")
	e.write("crlf.txt", "a\r\nb\r\n")
	for _, f := range []string{"missing.txt", "empty.txt", "crlf.txt", "../out.txt", "/etc/passwd"} {
		notes := e.hook(HookRequest{Selects: []HookSelect{{File: f, Range: HookRange{Mode: RangeAll}, Tool: "Read"}}})
		if len(notes) != 1 {
			t.Errorf("%s: notes = %v", f, notes)
		}
	}
	for _, k := range e.kinds() {
		if k == tape.TypeSelect {
			t.Error("a select was recorded for a file that cannot be recorded")
		}
	}
}

func TestHookSelectSeesAnExternalChange(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "a\nb\n")
	e.hook(HookRequest{Selects: []HookSelect{{File: "f.txt", Range: HookRange{Mode: RangeAll}, Tool: "Read"}}})
	e.write("f.txt", "a\nB\nc\n")
	e.hook(HookRequest{Selects: []HookSelect{{File: "f.txt", Range: HookRange{Mode: RangeAll}, Tool: "Read"}}})
	if got, want := e.kinds(), []string{"snapshot", "select", "external", "snapshot", "select"}; !reflect.DeepEqual(got, want) {
		t.Errorf("kinds = %v, want %v", got, want)
	}
	for _, ev := range e.events() {
		if ev.Type == tape.TypeExternal && ev.DetectedBy != "hook" {
			t.Errorf("detectedBy = %q", ev.DetectedBy)
		}
	}
	e.checkTape()
}

func TestObserveAllFindsWhatBashChanged(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", "1\n")
	e.write("b.txt", "2\n")
	e.write("c.txt", "3\n")
	for _, f := range []string{"a.txt", "b.txt", "c.txt"} {
		e.hook(HookRequest{Selects: []HookSelect{{File: f, Range: HookRange{Mode: RangeAll}, Tool: "Read"}}})
	}
	e.write("a.txt", "one\n")
	e.write("c.txt", "")
	if err := removeFile(e.root, "b.txt"); err != nil {
		t.Fatal(err)
	}
	before := len(e.events())
	e.hook(HookRequest{ObserveAll: true})
	var ext []string
	for _, ev := range e.events()[before:] {
		if ev.Type == tape.TypeExternal {
			ext = append(ext, ev.File)
			if ev.File == "b.txt" && !ev.Deleted {
				t.Error("b.txt is gone and must be recorded as deleted")
			}
		}
	}
	if !reflect.DeepEqual(ext, []string{"a.txt", "b.txt", "c.txt"}) {
		t.Errorf("external files = %v", ext)
	}
	// nothing changed since: nothing is recorded
	n := len(e.events())
	e.hook(HookRequest{ObserveAll: true})
	if len(e.events()) != n {
		t.Error("a second look at unchanged files recorded something")
	}
}

// edit makes the change an Edit would make to the file, then tells the hook.
func (e *env) edit(rel, old, repl string, all bool, original *string) []tape.Event {
	e.t.Helper()
	cur := e.read(rel)
	n := 1
	if all {
		n = -1
	}
	e.write(rel, strings.Replace(cur, old, repl, n))
	before := max(e.eventCount(), 1) // the header is not an event of the edit
	e.hook(HookRequest{Edit: &HookEdit{File: rel, OldString: old, NewString: repl, ReplaceAll: all, Original: original}})
	return e.events()[before:]
}

func TestHookEdit(t *testing.T) {
	type want struct {
		a, b, na, nb int
		old, new     string
	}
	for name, tc := range map[string]struct {
		text, old, repl string
		all             bool
		reps            []want
	}{
		"one line":         {"a\nfoo bar\nc\n", "foo", "baz", false, []want{{2, 2, 2, 2, "foo bar", "baz bar"}}},
		"adds lines":       {"a\nb\nc\n", "b", "b1\nb2", false, []want{{2, 2, 2, 3, "b", "b1\nb2"}}},
		"removes a line":   {"a\nb\nc\n", "b\n", "", false, []want{{2, 2, 2, 1, "b", ""}}},
		"several lines":    {"a\nb\nc\nd\n", "b\nc", "X", false, []want{{2, 3, 2, 2, "b\nc", "X"}}},
		"inside a line":    {"x = 1\n", "1", "2", false, []want{{1, 1, 1, 1, "x = 1", "x = 2"}}},
		"the last line":    {"a\nb", "b", "c", false, []want{{2, 2, 2, 2, "b", "c"}}},
		"a blank line":     {"a\n\nb\n", "\n\n", "\n", false, []want{{1, 2, 1, 1, "a\n", "a"}}},
		"every match":      {"x\ny\nx\nz\nx\n", "x", "w", true, []want{{1, 1, 1, 1, "x", "w"}, {3, 3, 3, 3, "x", "w"}, {5, 5, 5, 5, "x", "w"}}},
		"every match grow": {"x\nx\n", "x", "x\nx", true, []want{{1, 1, 1, 2, "x", "x\nx"}, {3, 3, 3, 4, "x", "x\nx"}}},
	} {
		e := newEnv(t)
		e.write("f.txt", tc.text)
		// the tape knows the file from an earlier look
		e.hook(HookRequest{Selects: []HookSelect{{File: "f.txt", Range: HookRange{Mode: RangeAll}, Tool: "Read"}}})
		evs := e.edit("f.txt", tc.old, tc.repl, tc.all, nil)
		var got []want
		for _, ev := range evs {
			if ev.Type != tape.TypeReplace {
				t.Errorf("%s: %s recorded after the edit", name, ev.Type)
				continue
			}
			if ev.Source != tape.SourceHook || ev.HookTool != "Edit" || ev.Why != nil || ev.From != nil || ev.Selection != nil {
				t.Errorf("%s: a hook replace has source hook, tool Edit, no why, no tokens: %+v", name, ev)
			}
			got = append(got, want{ev.StartLine, ev.EndLine, ev.NewStartLine, ev.NewEndLine, ev.OldText, ev.NewText})
		}
		if !reflect.DeepEqual(got, tc.reps) {
			t.Errorf("%s:\n got %+v\nwant %+v", name, got, tc.reps)
		}
		e.checkTape()
	}
}

func TestHookEditNeedsOnlyTheOriginalWhenTheTapeKnowsNothing(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "a\nb\nc\n")
	orig := "a\nb\nc\n"
	evs := e.edit("f.txt", "b", "B", false, &orig)
	var kinds []string
	for _, ev := range evs {
		kinds = append(kinds, ev.Type)
	}
	if !reflect.DeepEqual(kinds, []string{"snapshot", "replace"}) {
		t.Fatalf("kinds = %v", kinds)
	}
	if evs[0].Text == nil || *evs[0].Text != orig {
		t.Errorf("the snapshot must hold the file before the edit: %v", evs[0].Text)
	}
	e.checkTape()
}

func TestHookEditWithoutAnyKnowledgeRecordsTheFileAsItIs(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "a\nb\n")
	e.write("f.txt", "a\nB\n")
	notes := e.hook(HookRequest{Edit: &HookEdit{File: "f.txt", OldString: "b", NewString: "B"}})
	if got := e.kinds(); !reflect.DeepEqual(got, []string{"snapshot"}) || len(notes) != 1 {
		t.Errorf("kinds = %v, notes = %v", got, notes)
	}
	e.checkTape()
}

func TestHookEditThatDoesNotMatchTheFileIsExternal(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "a\nb\nc\n")
	e.hook(HookRequest{Selects: []HookSelect{{File: "f.txt", Range: HookRange{Mode: RangeAll}, Tool: "Read"}}})
	// the file now is not what the edit makes of the old content (something else changed it as well)
	e.write("f.txt", "a\nB\nc\nextra\n")
	before := len(e.events())
	e.hook(HookRequest{Edit: &HookEdit{File: "f.txt", OldString: "b", NewString: "B"}})
	var kinds []string
	for _, ev := range e.events()[before:] {
		kinds = append(kinds, ev.Type)
	}
	if !reflect.DeepEqual(kinds, []string{"external", "snapshot"}) {
		t.Errorf("kinds = %v", kinds)
	}
	e.checkTape()
}

func TestHookEditKeepsATokenOfMcpValid(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "a\nb\nc\nd\ne\n")
	low := e.sel(e.c, "f.txt", 4, 5) // d and e
	// the agent's Edit adds two lines above
	e.edit("f.txt", "a", "a\nx\ny", false, nil)
	res := e.rep(e.core(), low.Selection, "D\nE")
	if res.StartLine != 6 || res.EndLine != 7 {
		t.Errorf("the token was corrected to %d..%d, want 6..7", res.StartLine, res.EndLine)
	}
	if got := e.read("f.txt"); got != "a\nx\ny\nb\nc\nD\nE\n" {
		t.Errorf("file = %q", got)
	}
	e.checkTape()
}

func TestEditStepsRefusesWhatLinesCannotTell(t *testing.T) {
	for name, tc := range map[string]struct{ text, old, repl string }{
		"empty old":               {"a\n", "", "x"},
		"no match":                {"a\n", "z", "x"},
		"a newline added at end":  {"a", "a", "a\n"},
		"a final newline removed": {"a\n", "a\n", "a"},
	} {
		if _, _, ok := editSteps(tc.text, tc.old, tc.repl, false); ok {
			t.Errorf("%s: accepted", name)
		}
	}
}

func removeFile(root, rel string) error { return os.Remove(filepath.Join(root, rel)) }

// eventCount is the number of events on the tape, 0 before there is one.
func (e *env) eventCount() int {
	if _, err := os.Stat(filepath.Join(e.root, ".srwr", "active")); err != nil {
		return 0
	}
	return len(e.events())
}
