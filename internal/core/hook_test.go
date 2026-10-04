package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
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
	if got, want := e.kinds(), []string{"snapshot", "select", "external", "select"}; !reflect.DeepEqual(got, want) {
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
	if !reflect.DeepEqual(kinds, []string{"external"}) {
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

// gitInit makes the workspace a git work tree (no commit needed: new files are the ones git lists as untracked).
func (e *env) gitInit() {
	e.t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		e.t.Skip("git is not installed")
	}
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = e.root
	if out, err := cmd.CombinedOutput(); err != nil {
		e.t.Fatalf("git init: %v\n%s", err, out)
	}
}

func (e *env) bash() []string { e.t.Helper(); return e.hook(HookRequest{ObserveAll: true}) }

func (e *env) createdFiles() map[string]tape.Event {
	got := map[string]tape.Event{}
	for _, ev := range e.events() {
		if ev.Type == tape.TypeExternal && ev.Created {
			got[ev.File] = ev
		}
	}
	return got
}

// A file a Bash command made is on the tape: an external with Created, told as lines added to nothing.
func TestHookRecordsANewFileMadeByBash(t *testing.T) {
	e := newEnv(t)
	e.gitInit()
	e.write("seen.txt", "x\n")
	e.sel(e.c, "seen.txt", 1, 1)
	e.write("dir/new.go", "package a\n\nfunc f() {}\n")
	e.write("empty.txt", "")
	e.bash()

	got := e.createdFiles()
	n := got["dir/new.go"]
	if len(got) != 2 || n.Hunks == nil || n.Text != nil || n.ExpectedSha != "" || n.ActualSha != tape.Sha("package a\n\nfunc f() {}\n") ||
		n.DetectedBy != "hook" || n.Author == nil || n.Author.Kind != "external" {
		t.Fatalf("created = %+v", got)
	}
	if ev := got["empty.txt"]; ev.Text == nil || *ev.Text != "" {
		t.Errorf("an empty new file = %+v, want an empty text", ev)
	}
	st := tape.Build(e.events())
	if st.Files["dir/new.go"] == nil || st.Files["dir/new.go"].Text != e.read("dir/new.go") || st.Files["empty.txt"] == nil {
		t.Errorf("the tape does not know the new files: %+v", st.Files)
	}

	// The next Bash does not record them again, and a later change is an ordinary external.
	before := e.eventCount()
	e.bash()
	if e.eventCount() != before {
		t.Errorf("a second Bash added %d events", e.eventCount()-before)
	}
	e.write("dir/new.go", "package a\n\nfunc g() {}\n")
	e.bash()
	evs := e.events()
	last := evs[len(evs)-1]
	if last.Type != tape.TypeExternal || last.Created || last.Hunks == nil || last.File != "dir/new.go" {
		t.Errorf("the change = %+v", last)
	}
	e.checkTape()
}

// A command that makes a file and adds it to git in the same go (git add, git add -N) still leaves its content on the tape.
func TestHookRecordsANewFileThatBashAddedToGit(t *testing.T) {
	e := newEnv(t)
	e.gitInit()
	e.write("seen.txt", "x\n")
	e.sel(e.c, "seen.txt", 1, 1)
	e.write("intent.go", "package a\n")
	e.write("staged.go", "package b\n")
	for _, args := range [][]string{{"add", "-N", "intent.go"}, {"add", "staged.go"}} {
		cmd := exec.Command("git", args...) //nolint:gosec // git, with fixed arguments, in a temporary directory
		cmd.Dir = e.root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	e.bash()
	got := e.createdFiles()
	if got["intent.go"].File == "" || got["staged.go"].File == "" || len(got) != 2 {
		t.Fatalf("created = %+v", got)
	}
	e.checkTape()
}

// Entrance 5: the new files are looked for through readTarget, so what is not recorded is left out, and so is what cannot be handled.
func TestHookNewFilesLeaveOutWhatIsNotRecorded(t *testing.T) {
	e := newEnv(t)
	e.gitInit()
	e.write(".srwrignore", "priv*.txt\n")
	e.write(".gitignore", "built.txt\n")
	e.write("private.txt", secret)
	e.write(".env", secret)
	e.write("built.txt", "x\n")
	e.write("crlf.txt", "a\r\nb\r\n")
	e.write("bin.dat", "a\x00b")
	e.write("big.txt", strings.Repeat("x\n", maxNewFileSize))
	e.write("ok.txt", "fine\n")
	_ = os.Symlink(filepath.Join(e.root, "ok.txt"), filepath.Join(e.root, "link.txt")) // a link to a file inside is fine; where links are not allowed it is just missing
	e.bash()
	got := e.createdFiles()
	for name := range got {
		switch name {
		case "ok.txt", "link.txt", ".gitignore", ".srwrignore": // link.txt is a link to a file inside the workspace
		default:
			t.Errorf("%s was recorded", name)
		}
	}
	if got["ok.txt"].File == "" {
		t.Error("ok.txt was not recorded")
	}
	e.noSecretOnTape(".env", "private.txt")
}

func TestHookNewFilesStopAtTheLimit(t *testing.T) {
	e := newEnv(t)
	e.gitInit()
	for i := range maxNewFiles + 7 {
		e.write(filepath.Join("gen", strings.Repeat("a", 1)+strconv.Itoa(1000+i)+".txt"), "x\n")
	}
	notes := e.bash()
	if got := len(e.createdFiles()); got != maxNewFiles {
		t.Errorf("%d new files recorded, want %d", got, maxNewFiles)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "7 more new files") {
		t.Errorf("notes = %v", notes)
	}
}

// Outside a git work tree there is no list of new files: nothing happens, and the agent is not stopped.
func TestHookNewFilesOutsideGit(t *testing.T) {
	e := newEnv(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	if exec.Command("git", "-C", e.root, "rev-parse", "--git-dir").Run() == nil { //nolint:gosec // git, in a temporary directory
		t.Skip("the temporary directory is inside a git work tree")
	}
	e.write("new.txt", "x\n")
	if notes := e.bash(); len(notes) != 0 {
		t.Errorf("notes %v, want none", notes)
	}
	if _, err := os.Stat(filepath.Join(e.root, ".srwr", "active")); err == nil {
		t.Error("a tape was made although nothing was found")
	}
}
