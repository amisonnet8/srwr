package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/amisonnet8/srwr/internal/tape"
)

// runHookProcess starts the real `srwr hook` the way Claude Code does (the JSON on the standard input) and returns what it
// wrote on the standard error output. The exit code must be 0: anything else would be seen by the agent.
func runHookProcess(t *testing.T, root string, payload map[string]any) string {
	t.Helper()
	payload["hook_event_name"] = "PostToolUse"
	payload["cwd"] = root
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary(t), "hook") //nolint:gosec // the binary was built by this test
	cmd.Dir = root                         // no --root: the working directory is the workspace, like a hook started in the project
	cmd.Stdin = bytes.NewReader(b)
	var errOut bytes.Buffer
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		t.Fatalf("srwr hook: %v (%s)", err, errOut.String())
	}
	return errOut.String()
}

func toolPayload(tool string, input, response any) map[string]any {
	return map[string]any{"tool_name": tool, "tool_input": input, "tool_response": response}
}

// Stage condition: what the agent looked at and edited with its own tools lands in the same tape as select and replace, and a
// token srwr mcp issued before the agent's own Edit is still right after it.
func TestHookAndMCPWriteOneTape(t *testing.T) {
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	text := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(1)\n}\n"
	write(t, root, "main.go", text)
	m := startClient(t, root)
	m.initialize()
	file := filepath.Join(root, "main.go")

	low := m.mustSelect("main.go", 5, 7) // func main() … }

	// The agent reads the file, searches it, and edits it with its own tools.
	runHookProcess(t, root, toolPayload("Read", map[string]any{"file_path": file, "offset": 1, "limit": 4}, map[string]any{"type": "text"}))
	runHookProcess(t, root, toolPayload("Bash", map[string]any{"command": "grep -n Println main.go"}, map[string]any{"stdout": "6:\tfmt.Println(1)\n"}))
	runHookProcess(t, root, toolPayload("Grep", map[string]any{"pattern": "func", "output_mode": "content", "path": file}, map[string]any{"content": "5:func main() {\n"}))
	edited := strings.Replace(text, "import \"fmt\"\n", "import (\n\t\"fmt\"\n\t\"os\"\n)\n", 1)
	write(t, root, "main.go", edited)
	runHookProcess(t, root, toolPayload("Edit",
		map[string]any{"file_path": file, "old_string": "import \"fmt\"", "new_string": "import (\n\t\"fmt\"\n\t\"os\"\n)"},
		map[string]any{"filePath": file, "originalFile": text, "replaceAll": false}))

	// The token srwr mcp issued before the Edit still points at func main (it moved down by three lines).
	m.mustReplace(low, "func main() {\n\tfmt.Println(2)\n}")
	if got, want := read(t, root, "main.go"), strings.Replace(edited, "Println(1)", "Println(2)", 1); got != want {
		t.Errorf("main.go = %q, want %q", got, want)
	}

	files := tapes(t, root)
	if len(files) != 1 {
		t.Fatalf("tapes = %v, want one", files)
	}
	events := readTape(t, files[0])
	checkSeqs(t, events)
	var got []string
	for _, e := range events[1:] {
		switch e.Type {
		case tape.TypeLook, tape.TypeEdit:
			src := e.Source
			if src == "" {
				src = "mcp"
			}
			got = append(got, fmt.Sprintf("%s/%s/%s:%d-%d", e.Type, src, e.HookTool, e.StartLine, e.EndLine))
		default:
			got = append(got, e.Type)
		}
	}
	want := []string{
		"snapshot", "look/mcp/:5-7",
		"look/hook/Read:1-4", "look/hook/Bash:6-6", "look/hook/Grep:5-5",
		"edit/hook/Edit:3-3", "edit/mcp/:8-10",
	}
	if !slices.Equal(got, want) {
		t.Errorf("tape:\n got %v\nwant %v", got, want)
	}
	for _, e := range events[1:] {
		if e.Source == tape.SourceHook && (e.Why != nil || e.Selection != nil || e.From != nil) {
			t.Errorf("a hook event has a why or a token: %+v", e)
		}
	}
	if st := tape.Build(events); st.Files["main.go"].Text != read(t, root, "main.go") {
		t.Error("the tape does not replay to the file")
	}
}

// Several hook processes and a srwr mcp write together: nothing is lost or torn, the numbers have no gaps.
func TestHookProcessesAndMCPWriteTogether(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "1\n2\n3\n")
	write(t, root, "b.txt", "x\ny\n")
	m := startClient(t, root)
	m.initialize()
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f := "a.txt"
			if i%2 == 1 {
				f = "b.txt"
			}
			runHookProcess(t, root, toolPayload("Read", map[string]any{"file_path": filepath.Join(root, f)}, nil))
		}()
	}
	for range 4 {
		m.mustSelect("a.txt", 1, 2)
	}
	wg.Wait()
	files := tapes(t, root)
	if len(files) != 1 {
		t.Fatalf("tapes = %v, want one", files)
	}
	events := readTape(t, files[0])
	checkSeqs(t, events)
	n := map[string]int{}
	for _, e := range events[1:] {
		if e.Type == tape.TypeLook {
			n[e.Source]++
		}
	}
	if n[tape.SourceHook] != 12 || n[tape.SourceMCP] != 4 {
		t.Errorf("selects by source = %v, want 12 from hook and 4 from mcp", n)
	}
}

// An agent that works without srwr mcp at all (hooks only) still gets a tape, and the viewer shows it without a why.
func TestViewServerShowsWhatHookWrote(t *testing.T) {
	root := t.TempDir()
	write(t, root, "main.go", "package main\n\nfunc main() {\n\trun()\n}\n")
	file := filepath.Join(root, "main.go")
	runHookProcess(t, root, toolPayload("Read", map[string]any{"file_path": file}, nil))
	write(t, root, "main.go", "package main\n\nfunc main() {\n\tsetup()\n\trun()\n}\n")
	runHookProcess(t, root, toolPayload("Edit",
		map[string]any{"file_path": file, "old_string": "\trun()", "new_string": "\tsetup()\n\trun()"},
		map[string]any{"originalFile": "package main\n\nfunc main() {\n\trun()\n}\n"}))
	events := readTape(t, tapes(t, root)[0])
	if h := events[0]; h.Author == nil || h.Author.Name != "claude" || h.Author.Kind != "ai" {
		t.Errorf("header author = %+v", h.Author)
	}

	v := startViewer(t, root)
	v.request("initialize", `{"client":"vim","protocolVersion":2}`)
	var list struct {
		Result struct{ Tapes []struct{ TapeID string } }
	}
	if err := json.Unmarshal([]byte(v.request("tapes/list", `{}`)), &list); err != nil || len(list.Result.Tapes) != 1 {
		t.Fatalf("tapes/list: %+v %v", list, err)
	}
	var opened struct {
		Result struct {
			Frames []struct {
				Kind          string
				Before, After string
				Range         struct{ Start, End int }
				Why           *string
			}
		}
	}
	line := v.request("tape/open", fmt.Sprintf(`{"tapeId":%q,"withText":true}`, list.Result.Tapes[0].TapeID))
	if err := json.Unmarshal([]byte(line), &opened); err != nil || len(opened.Result.Frames) != 2 {
		t.Fatalf("tape/open: %s %v", line, err)
	}
	sel, rep := opened.Result.Frames[0], opened.Result.Frames[1]
	if sel.Kind != "look" || sel.Why != nil || sel.Range.Start != 1 || sel.Range.End != 5 {
		t.Errorf("select frame = %+v", sel)
	}
	if rep.Kind != "edit" || rep.Why != nil || rep.Range.Start != 4 || rep.Range.End != 5 || !strings.Contains(rep.After, "setup()") {
		t.Errorf("replace frame = %+v", rep)
	}
}

// A file a Bash command made is recorded by the real srwr hook, and srwr tapes check has nothing to say about it.
func TestHookRecordsANewFileFromBash(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	runGit(t, root, "init", "-q")
	write(t, root, "main.go", "package main\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "first")
	runHookProcess(t, root, toolPayload("Read", map[string]any{"file_path": filepath.Join(root, "main.go")}, nil))
	write(t, root, "util/helper.go", "package util\n\nfunc Help() {}\n") // as `cat > util/helper.go` would
	runHookProcess(t, root, toolPayload("Bash", map[string]any{"command": "cat > util/helper.go <<EOF"}, map[string]any{"stdout": ""}))

	events := readTape(t, tapes(t, root)[0])
	var created []tape.Event
	for _, e := range events {
		if e.Type == tape.TypeExternal && e.Created {
			created = append(created, e)
		}
	}
	if len(created) != 1 || created[0].File != "util/helper.go" || created[0].Hunks == nil {
		t.Fatalf("created = %+v", created)
	}
	if st := tape.Build(events); st.Files["util/helper.go"] == nil || st.Files["util/helper.go"].Text != read(t, root, "util/helper.go") {
		t.Error("the tape does not replay to the new file")
	}
	english(t)
	if code, out, errOut := tapesOut(t, root, "check"); code != 0 {
		t.Errorf("tapes check: code %d, stderr %q\n%s", code, errOut, out)
	}
}

// After a Read, srwr hook writes the JSON Claude Code shows the agent on the standard output, and exits with 0.
func TestHookTellsTheAgentAboutLook(t *testing.T) {
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	write(t, root, "main.go", "package main\n")
	for _, tc := range []struct {
		tool  string
		input map[string]any
		want  bool
	}{
		{"Read", map[string]any{"file_path": filepath.Join(root, "main.go")}, true},
		{"Grep", map[string]any{"pattern": "main", "output_mode": "content", "path": filepath.Join(root, "main.go")}, false},
	} {
		payload := toolPayload(tc.tool, tc.input, map[string]any{"content": "1:package main\n"})
		payload["hook_event_name"], payload["cwd"] = "PostToolUse", root
		b, _ := json.Marshal(payload)
		cmd := exec.Command(binary(t), "hook") //nolint:gosec // the binary was built by this test
		cmd.Dir = root
		cmd.Stdin = bytes.NewReader(b)
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
		var got struct {
			Out struct {
				Event   string `json:"hookEventName"`
				Context string `json:"additionalContext"`
			} `json:"hookSpecificOutput"`
		}
		if !tc.want {
			if out.Len() != 0 {
				t.Errorf("%s: stdout = %q, want nothing", tc.tool, out.String())
			}
			continue
		}
		if err := json.Unmarshal(out.Bytes(), &got); err != nil || got.Out.Event != "PostToolUse" || !strings.Contains(got.Out.Context, "look") {
			t.Errorf("%s: stdout = %q (%v)", tc.tool, out.String(), err)
		}
	}
}
