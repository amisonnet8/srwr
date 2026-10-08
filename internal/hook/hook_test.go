package hook

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/core"
	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/tape"
)

type env struct {
	t    *testing.T
	root string
	c    *core.Core
}

func newEnv(t *testing.T) *env {
	t.Helper()
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	ws, err := session.Open(root, session.Options{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return &env{t: t, root: root, c: &core.Core{WS: ws}}
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

// call runs the hook with a JSON of the shape Claude Code sends (PostToolUse).
func (e *env) call(tool string, toolInput, toolResponse any) []string {
	e.t.Helper()
	msg := map[string]any{"hook_event_name": "PostToolUse", "session_id": "s", "cwd": e.root, "tool_name": tool, "tool_input": toolInput, "tool_response": toolResponse}
	b, err := json.Marshal(msg)
	if err != nil {
		e.t.Fatal(err)
	}
	notes, err := Run(strings.NewReader(string(b)), e.c)
	if err != nil {
		e.t.Fatal(err)
	}
	return notes
}

func (e *env) events() []tape.Event {
	e.t.Helper()
	active, err := os.ReadFile(filepath.Join(e.root, ".srwr", "active"))
	if err != nil {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(e.root, ".srwr", "tapes", tape.FileName(strings.TrimSpace(string(active))))) //nolint:gosec // a path in a temporary directory
	if err != nil {
		e.t.Fatal(err)
	}
	var out []tape.Event
	for _, ev := range tape.Parse(b).Events {
		if ev.Type != tape.TypeHeader {
			out = append(out, ev)
		}
	}
	return out
}

// selects lists the hook selects as "tool file:a-b".
func (e *env) selects() []string {
	var out []string
	for _, ev := range e.events() {
		if ev.Type == tape.TypeLook {
			out = append(out, ev.HookTool+" "+ev.File+":"+itoa(ev.StartLine)+"-"+itoa(ev.EndLine))
		}
	}
	return out
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

const ten = "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"

func TestReadRecordsASelect(t *testing.T) {
	for name, tc := range map[string]struct {
		in   map[string]any
		want string
	}{
		"whole file":        {map[string]any{}, "Read f.txt:1-10"},
		"offset and limit":  {map[string]any{"offset": 3, "limit": 4}, "Read f.txt:3-6"},
		"offset only":       {map[string]any{"offset": 8}, "Read f.txt:8-10"},
		"limit only":        {map[string]any{"limit": 2}, "Read f.txt:1-2"},
		"numbers as string": {map[string]any{"offset": "2", "limit": "2"}, "Read f.txt:2-3"},
	} {
		e := newEnv(t)
		e.write("f.txt", ten)
		in := tc.in
		in["file_path"] = filepath.Join(e.root, "f.txt")
		e.call("Read", in, map[string]any{"type": "text", "file": map[string]any{"filePath": in["file_path"], "content": ten}})
		if got := e.selects(); !reflect.DeepEqual(got, []string{tc.want}) {
			t.Errorf("%s: %v, want %s", name, got, tc.want)
		}
	}
}

func TestReadOutsideTheWorkspaceIsLeftOut(t *testing.T) {
	e := newEnv(t)
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "x.txt"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	notes := e.call("Read", map[string]any{"file_path": filepath.Join(other, "x.txt")}, nil)
	if len(notes) != 1 || len(e.events()) != 0 {
		t.Errorf("notes = %v, events = %v", notes, e.events())
	}
}

func TestEditRecordsAReplace(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "a\nfoo\nc\n")
	e.write("f.txt", "a\nbar\nc\n") // the agent's Edit has been done
	e.call("Edit",
		map[string]any{"file_path": filepath.Join(e.root, "f.txt"), "old_string": "foo", "new_string": "bar", "replace_all": false},
		map[string]any{"filePath": filepath.Join(e.root, "f.txt"), "oldString": "foo", "newString": "bar", "originalFile": "a\nfoo\nc\n", "replaceAll": false, "userModified": false})
	evs := e.events()
	if len(evs) != 2 || evs[0].Type != tape.TypeSnapshot || evs[1].Type != tape.TypeEdit {
		t.Fatalf("events = %+v", evs)
	}
	r := evs[1]
	if r.File != "f.txt" || r.StartLine != 2 || r.EndLine != 2 || r.OldText != "foo" || r.NewText != "bar" || r.Source != tape.SourceHook || r.HookTool != "Edit" {
		t.Errorf("replace = %+v", r)
	}
}

func TestBashRecordsAReadAndLooksAtEveryFile(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", ten)
	e.write("g.txt", "g\n")
	e.call("Bash", map[string]any{"command": "cat g.txt"}, map[string]any{"stdout": "g\n", "stderr": ""})
	e.call("Bash", map[string]any{"command": "sed -n '2,4p' f.txt"}, map[string]any{"stdout": "2\n3\n4\n"})
	if got := e.selects(); !reflect.DeepEqual(got, []string{"Bash g.txt:1-1", "Bash f.txt:2-4"}) {
		t.Errorf("selects = %v", got)
	}
	// a Bash that is not a read still looks at the files the tape knows
	e.write("g.txt", "changed\n")
	e.call("Bash", map[string]any{"command": "make build"}, map[string]any{"stdout": "ok"})
	var ext []string
	for _, ev := range e.events() {
		if ev.Type == tape.TypeExternal {
			ext = append(ext, ev.File+" by "+ev.DetectedBy)
		}
	}
	if !reflect.DeepEqual(ext, []string{"g.txt by hook"}) {
		t.Errorf("external = %v", ext)
	}
}

func TestBashGrepUsesTheNumbersOfItsOutput(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", ten)
	e.call("Bash", map[string]any{"command": "grep -n 1 f.txt"}, map[string]any{"stdout": "1:1\n10:10\n"})
	if got := e.selects(); !reflect.DeepEqual(got, []string{"Bash f.txt:1-1", "Bash f.txt:10-10"}) {
		t.Errorf("selects = %v", got)
	}
}

func TestGrepContentModeRecordsRuns(t *testing.T) {
	for name, resp := range map[string]any{
		"object with content": map[string]any{"mode": "content", "numFiles": 2, "content": "a.txt:2:x\na.txt:3:y\nsub/b.txt:7:z\n", "numLines": 3},
		"plain text":          "a.txt:2:x\na.txt:3:y\nsub/b.txt:7:z\n",
		"absolute names":      "ROOT/a.txt:2:x\nROOT/a.txt-3-y\nROOT/sub/b.txt:7:z\n",
	} {
		e := newEnv(t)
		e.write("a.txt", ten)
		e.write("sub/b.txt", ten)
		if s, ok := resp.(string); ok {
			resp = strings.ReplaceAll(s, "ROOT", e.root)
		}
		e.call("Grep", map[string]any{"pattern": "x", "output_mode": "content", "-n": true}, resp)
		if got, want := e.selects(), []string{"Grep a.txt:2-3", "Grep sub/b.txt:7-7"}; !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %v, want %v", name, got, want)
		}
	}
}

func TestGrepInOneFileHasNoNamesInItsLines(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", ten)
	e.call("Grep", map[string]any{"pattern": "x", "path": filepath.Join(e.root, "a.txt"), "output_mode": "content"}, map[string]any{"content": "4:x\n5:y\n9:z\n"})
	if got, want := e.selects(), []string{"Grep a.txt:4-5", "Grep a.txt:9-9"}; !reflect.DeepEqual(got, want) {
		t.Errorf("%v, want %v", got, want)
	}
}

func TestAGrepWithManyMatchesIsCutShort(t *testing.T) {
	e := newEnv(t)
	var text, out strings.Builder
	for i := 1; i <= 300; i++ {
		text.WriteString("x\n")
		if i%2 == 1 { // every other line: no two of them are next to each other
			out.WriteString("a.txt:" + itoa(i) + ":x\n")
		}
	}
	e.write("a.txt", text.String())
	notes := e.call("Grep", map[string]any{"pattern": "x", "output_mode": "content"}, out.String())
	if got := len(e.selects()); got != maxSelects {
		t.Errorf("%d selects, want %d", got, maxSelects)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "100") {
		t.Errorf("notes = %v", notes)
	}
}

func TestGrepThatListsOnlyFilesRecordsNothing(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", ten)
	e.call("Grep", map[string]any{"pattern": "x", "output_mode": "files_with_matches"}, map[string]any{"filenames": []string{"a.txt"}})
	e.call("Grep", map[string]any{"pattern": "x"}, map[string]any{"filenames": []string{"a.txt"}})
	if len(e.events()) != 0 {
		t.Errorf("events = %+v", e.events())
	}
}

func TestOtherCallsAreLeftAlone(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", ten)
	for _, tool := range []string{"Write", "MultiEdit", "NotebookEdit", "Glob", "WebFetch", "Task", "mcp__srwr__look"} {
		e.call(tool, map[string]any{"file_path": filepath.Join(e.root, "f.txt"), "content": "x"}, nil)
	}
	preJSON, err := json.Marshal(map[string]any{"hook_event_name": "PreToolUse", "cwd": e.root, "tool_name": "Read", "tool_input": map[string]any{"file_path": filepath.Join(e.root, "f.txt")}})
	if err != nil {
		t.Fatal(err)
	}
	pre := string(preJSON)
	if _, err := Run(strings.NewReader(pre), e.c); err != nil {
		t.Fatal(err)
	}
	if len(e.events()) != 0 {
		t.Errorf("events = %+v", e.events())
	}
}

func TestBrokenInputIsAnErrorForTheCaller(t *testing.T) {
	e := newEnv(t)
	for _, in := range []string{"", "not json", `{"tool_name":`} {
		if _, err := Run(strings.NewReader(in), e.c); err == nil {
			t.Errorf("%q was accepted", in)
		}
	}
	// a call whose input has the wrong shape is noted, not an error
	notes := e.call("Read", "a string instead of an object", nil)
	if len(notes) != 1 {
		t.Errorf("notes = %v", notes)
	}
}

func TestRelativePathsAreRelativeToWhereTheAgentWas(t *testing.T) {
	e := newEnv(t)
	e.write("sub/f.txt", ten)
	b, _ := json.Marshal(map[string]any{"hook_event_name": "PostToolUse", "cwd": filepath.Join(e.root, "sub"), "tool_name": "Read", "tool_input": map[string]any{"file_path": "f.txt"}})
	if _, err := Run(strings.NewReader(string(b)), e.c); err != nil {
		t.Fatal(err)
	}
	if got := e.selects(); !reflect.DeepEqual(got, []string{"Read sub/f.txt:1-10"}) {
		t.Errorf("selects = %v", got)
	}
}

func TestToRel(t *testing.T) {
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	out := t.TempDir()
	for name, tc := range map[string]struct {
		cwd, p string
		want   string
		ok     bool
	}{
		"inside":         {root, filepath.Join(root, "a", "b.go"), "a/b.go", true},
		"relative":       {filepath.Join(root, "a"), "b.go", "a/b.go", true},
		"no cwd":         {"", "b.go", "b.go", true},
		"a file to come": {root, filepath.Join(root, "new", "x.go"), "new/x.go", true},
		"outside":        {root, filepath.Join(out, "x"), "", false},
		"dot dot":        {root, "../x", "", false},
		"the root":       {root, root, "", false},
		"empty":          {root, "", "", false},
	} {
		got, ok := toRel(root, tc.cwd, tc.p)
		if got != tc.want || ok != tc.ok {
			t.Errorf("%s: %q %v, want %q %v", name, got, ok, tc.want, tc.ok)
		}
	}
	// a symbolic link inside the workspace that points out of it
	if err := os.Symlink(out, filepath.Join(root, "link")); err != nil {
		t.Skip("no symbolic links")
	}
	if got, ok := toRel(root, root, filepath.Join(root, "link", "x.go")); ok {
		t.Errorf("a path through a link to the outside was accepted: %q", got)
	}
}

// advice runs the hook and returns what the agent is to be told.
func (e *env) advice(tool string, toolInput, toolResponse any) []string {
	e.t.Helper()
	msg := map[string]any{"hook_event_name": "PostToolUse", "cwd": e.root, "tool_name": tool, "tool_input": toolInput, "tool_response": toolResponse}
	b, err := json.Marshal(msg)
	if err != nil {
		e.t.Fatal(err)
	}
	_, advice, err := RunWithAdvice(strings.NewReader(string(b)), e.c)
	if err != nil {
		e.t.Fatal(err)
	}
	return advice
}

// After one file is read the agent is told about look; after a search, or a read that was not recorded, it is not.
func TestLookAdvice(t *testing.T) {
	cases := []struct {
		name string
		tool string
		in   map[string]any
		resp any
		want bool
	}{
		{"Read", "Read", map[string]any{"file_path": "a.go"}, map[string]any{"type": "text"}, true},
		{"Read of lines", "Read", map[string]any{"file_path": "a.go", "offset": 1, "limit": 2}, map[string]any{"type": "text"}, true},
		{"cat", "Bash", map[string]any{"command": "cat a.go"}, map[string]any{"stdout": ""}, true},
		{"sed -n", "Bash", map[string]any{"command": "sed -n '1,2p' a.go"}, map[string]any{"stdout": ""}, true},
		{"head", "Bash", map[string]any{"command": "head -n 2 a.go"}, map[string]any{"stdout": ""}, true},
		{"grep -n of one file", "Bash", map[string]any{"command": "grep -n a a.go"}, map[string]any{"stdout": "1:a\n"}, false},
		{"Grep", "Grep", map[string]any{"pattern": "a", "output_mode": "content", "path": "a.go"}, map[string]any{"content": "1:a\n"}, false},
		{"a command that reads nothing", "Bash", map[string]any{"command": "ls"}, map[string]any{"stdout": ""}, false},
		{"Read outside the workspace", "Read", map[string]any{"file_path": "/etc/hostname"}, map[string]any{"type": "text"}, false},
		{"Read of a file that is not recorded", "Read", map[string]any{"file_path": ".env"}, map[string]any{"type": "text"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.write("a.go", "a\nb\nc\n")
			e.write(".env", "KEY=1\n")
			got := e.advice(tc.tool, tc.in, tc.resp)
			if (len(got) == 1 && got[0] == LookAdvice) != tc.want || (!tc.want && len(got) != 0) {
				t.Errorf("advice = %q, want advice %v", got, tc.want)
			}
		})
	}
}

// After a Bash command that made or changed files the agent is told, calmly; after one that changed nothing, or a Read, it is not.
func TestOutsideAdvice(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	e := newEnv(t)
	if out, err := exec.Command("git", "-C", e.root, "init", "-q").CombinedOutput(); err != nil { //nolint:gosec // git in a temporary directory
		t.Fatalf("git init: %v\n%s", err, out)
	}
	e.write("a.go", "a\nb\n")
	bash := func(cmd string) []string {
		return e.advice("Bash", map[string]any{"command": cmd}, map[string]any{"stdout": ""})
	}
	e.advice("Read", map[string]any{"file_path": "a.go"}, map[string]any{"type": "text"}) // the tape knows a.go
	if got := bash("ls"); len(got) != 0 {
		t.Errorf("a command that changed nothing: %q", got)
	}
	e.write("a.go", "a\nB\n")
	e.write("gen.go", "x\n")
	got := bash("go generate")
	if len(got) != 1 || !strings.Contains(got[0], "created gen.go and changed a.go") || !strings.Contains(got[0], "fine as they are") {
		t.Errorf("advice = %q", got)
	}
	if got := bash("ls"); len(got) != 0 {
		t.Errorf("the same changes again: %q", got)
	}
	if a := OutsideAdvice(core.OutsideChanges{Created: []string{"1", "2", "3", "4", "5", "6", "7"}}); !strings.Contains(a, "created 1, 2, 3, 4, 5 and 2 more.") {
		t.Errorf("advice = %q", a)
	}
}

// The advice to use look is given once for a tape: the second read (of any file) and a search before it get none.
func TestLookAdviceIsGivenOnce(t *testing.T) {
	e := newEnv(t)
	e.write("a.go", "a\nb\n")
	e.write("b.go", "c\nd\n")
	read := func(f string) []string {
		return e.advice("Read", map[string]any{"file_path": f}, map[string]any{"type": "text"})
	}
	if got := read("a.go"); len(got) != 1 || got[0] != LookAdvice {
		t.Fatalf("the first read: %q", got)
	}
	if got := read("b.go"); len(got) != 0 {
		t.Errorf("the second read: %q", got)
	}
	e2 := newEnv(t)
	e2.write("a.go", "a\nb\n")
	e2.advice("Grep", map[string]any{"pattern": "a", "output_mode": "content", "path": "a.go"}, map[string]any{"content": "1:a\n"})
	if got := e2.advice("Read", map[string]any{"file_path": "a.go"}, map[string]any{"type": "text"}); len(got) != 0 {
		t.Errorf("a read after a search: %q", got)
	}
}

// What the outside advice explains is said once; later it is a short line.
func TestOutsideAdviceIsShortAfterTheFirst(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	e := newEnv(t)
	if out, err := exec.Command("git", "-C", e.root, "init", "-q").CombinedOutput(); err != nil { //nolint:gosec // git in a temporary directory
		t.Fatalf("git init: %v\n%s", err, out)
	}
	e.write("a.go", "a\n")
	bash := func() []string {
		return e.advice("Bash", map[string]any{"command": "go generate"}, map[string]any{"stdout": ""})
	}
	e.advice("Read", map[string]any{"file_path": "a.go"}, map[string]any{"type": "text"})
	e.write("g1.go", "x\n")
	first := bash()
	e.write("g2.go", "y\n")
	second := bash()
	if len(first) != 1 || !strings.Contains(first[0], "fine as they are") {
		t.Errorf("first = %q", first)
	}
	if len(second) != 1 || second[0] != "srwr: this command created g2.go." {
		t.Errorf("second = %q", second)
	}
}
