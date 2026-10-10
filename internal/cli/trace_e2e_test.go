package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/tape"
)

// traceWorkspace makes a git work tree where the real srwr mcp changes a.go and makes b.go, a person adds a line to c.go, and all
// of it is committed. It returns the root and the text of git show for that commit.
func traceWorkspace(t *testing.T) (root, show string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root = t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	runGit(t, root, "init", "-q")
	write(t, root, "a.go", "package a\n\nfunc one() {}\n\nfunc two() {}\n")
	write(t, root, "c.go", "package c\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "first")

	m := startClient(t, root)
	m.initialize()
	if body, isErr := m.call("edit", map[string]any{"file": "a.go", "expect": "func one() {}", "newText": "func one() {\n\tprintln(\"one\")\n}", "why": "one に出力を足すため"}); isErr {
		t.Fatalf("edit: %v", body)
	}
	if body, isErr := m.call("new", map[string]any{"file": "b.go", "content": "package a\n\nfunc three() {\n\tprintln(\"three\")\n}\n", "why": "three を別のファイルに作るため"}); isErr {
		t.Fatalf("new: %v", body)
	}
	m.stop()
	write(t, root, "c.go", "package c\n\n// written by a person, not on a tape\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-q", "-m", "Make one, three and a note")
	return root, runGit(t, root, "show", "HEAD")
}

func runTraceBinary(t *testing.T, root, stdin string, env []string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(binary(t), append([]string{"trace", "--root", root}, args...)...) //nolint:gosec // the binary was built by this test
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = append(os.Environ(), env...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	code := 0
	if err != nil {
		code = 1
	}
	return out.String(), code
}

// Stage condition: srwr trace tells, for the lines a real commit adds, the tape operations that wrote them and their why, and
// counts the lines no tape holds.
func TestTraceOfAGitCommit(t *testing.T) {
	root, show := traceWorkspace(t)

	out, code := runTraceBinary(t, root, show, []string{"SRWR_LANG="})
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	for _, want := range []string{
		"commit ", "Make one, three and a note",
		"a.go", "one に出力を足すため", "edit ",
		"b.go", "three を別のファイルに作るため", "new ",
		"c.go", "not on a tape: 2 added lines",
		"3 files, 10 added lines: 8 from tapes, 2 not on a tape",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "#") {
		t.Errorf("no tape ID and seq:\n%s", out)
	}

	ja, _ := runTraceBinary(t, root, show, []string{"SRWR_LANG=ja"})
	if !strings.Contains(ja, "理由: one に出力を足すため") || !strings.Contains(ja, "テープに無い：追加 2 行") {
		t.Errorf("Japanese output:\n%s", ja)
	}

	// --tape with a tape that is not there: nothing is found.
	none, _ := runTraceBinary(t, root, show, []string{"SRWR_LANG="}, "--tape", "20200101-0000-none")
	if strings.Contains(none, "one に出力を足すため") {
		t.Errorf("--tape of another tape found a why:\n%s", none)
	}
}

func TestTraceMarkAndJSON(t *testing.T) {
	root, show := traceWorkspace(t)
	marked, code := runTraceBinary(t, root, show, nil, "--mark")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, marked)
	}
	// The text is all there, in the same order, and the why follows the @@ line of its hunk.
	var kept []string
	for _, l := range strings.Split(marked, "\n") {
		if !strings.HasPrefix(l, "# why: ") && !strings.HasPrefix(l, "# 理由: ") {
			kept = append(kept, l)
		}
	}
	if strings.TrimRight(strings.Join(kept, "\n"), "\n") != strings.TrimRight(show, "\n") {
		t.Errorf("--mark changed the text:\n%s", marked)
	}
	lines := strings.Split(marked, "\n")
	found := false
	for i, l := range lines {
		if strings.HasPrefix(l, "@@") && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "# ") {
			found = true
		}
	}
	if !found {
		t.Errorf("no why after an @@ line:\n%s", marked)
	}

	js, code := runTraceBinary(t, root, show, nil, "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, js)
	}
	var got struct {
		Commits []struct {
			Commit string
			Files  []struct {
				File     string
				Segments []struct {
					From, To int
					Tape     string
					Seq      int
					Why      string
				}
			}
		}
	}
	if err := json.Unmarshal([]byte(js), &got); err != nil || len(got.Commits) != 1 || len(got.Commits[0].Files) != 3 {
		t.Fatalf("json: %v\n%s", err, js)
	}
	a := got.Commits[0].Files[0]
	if a.File != "a.go" || len(a.Segments) == 0 || a.Segments[0].Tape == "" || a.Segments[0].Seq == 0 || !strings.Contains(a.Segments[0].Why, "one") {
		t.Errorf("a.go: %+v", a)
	}
}

// A text that is only a diff (no commit line) works too, and a text without any diff says so.
func TestTracePlainDiffAndNothing(t *testing.T) {
	root, _ := traceWorkspace(t)
	diff := runGit(t, root, "diff", "HEAD~1", "HEAD")
	out, code := runTraceBinary(t, root, diff, []string{"SRWR_LANG="})
	if code != 0 || strings.Contains(out, "commit ") || !strings.Contains(out, "one に出力を足すため") {
		t.Errorf("git diff: exit %d\n%s", code, out)
	}
	out, code = runTraceBinary(t, root, "hello\n", []string{"SRWR_LANG="})
	if code != 0 || !strings.Contains(out, "No diff found") {
		t.Errorf("no diff: exit %d\n%s", code, out)
	}
}

// Stage condition: --as-tape writes the operations of a commit as one tape that the display server opens, whose last content of
// a file is the file as the commit has it; the same input makes the same tape; and srwr trace does not count that tape as a source.
func TestTraceAsTape(t *testing.T) {
	root, show := traceWorkspace(t)
	before := tapes(t, root)

	out, code := runTraceBinary(t, root, show, []string{"SRWR_LANG="}, "--as-tape")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	i := strings.Index(out, "→ srwr view ")
	if i < 0 {
		t.Fatalf("no tape named in the output:\n%s", out)
	}
	id := strings.TrimSpace(out[i+len("→ srwr view "):])
	after := tapes(t, root)
	if len(after) != len(before)+1 {
		t.Fatalf("tapes before %v, after %v", before, after)
	}
	path := filepath.Join(root, ".srwr", "tapes", id+".tape.jsonl")
	events := readTape(t, path)
	if h := events[0]; h.Type != "header" || h.Author == nil || h.Author.Kind != "derived" {
		t.Errorf("header = %+v", h)
	}
	checkSeqs(t, events)
	last := map[string]string{}
	for _, e := range events[1:] {
		if e.File == "c.go" {
			t.Errorf("a file that no tape wrote is in the cut: %+v", e)
		}
		if e.Selection != nil {
			t.Errorf("a token is kept: %+v", e)
		}
	}
	// Replay the cut: the files are the ones the commit holds.
	st := buildState(events[1:])
	for _, f := range []string{"a.go", "b.go"} {
		last[f] = st[f]
		if strings.TrimSpace(last[f]) != strings.TrimSpace(read(t, root, f)) {
			t.Errorf("%s replayed = %q, file = %q", f, last[f], read(t, root, f))
		}
	}

	// The display server opens it.
	v := startViewer(t, root)
	v.request("initialize", `{"client":"vim","protocolVersion":3}`)
	var opened struct {
		Result struct {
			Frames []struct{ Kind, File, Why string }
		}
	}
	line := v.request("tape/open", `{"tapeId":"`+id+`"}`)
	if err := json.Unmarshal([]byte(line), &opened); err != nil || len(opened.Result.Frames) == 0 {
		t.Fatalf("tape/open: %v\n%s", err, line)
	}
	sawWhy := false
	for _, f := range opened.Result.Frames {
		if strings.Contains(f.Why, "one に出力を足すため") {
			sawWhy = true
		}
	}
	if !sawWhy {
		t.Errorf("no frame with the why:\n%s", line)
	}

	// The same input makes the same tape, and does not make a second one.
	b1, _ := os.ReadFile(path) //nolint:gosec // a path in a temporary directory
	out2, code := runTraceBinary(t, root, show, []string{"SRWR_LANG="}, "--as-tape")
	if code != 0 || !strings.Contains(out2, id) {
		t.Fatalf("again: exit %d\n%s", code, out2)
	}
	b2, _ := os.ReadFile(path) //nolint:gosec // a path in a temporary directory
	if !bytes.Equal(b1, b2) || len(tapes(t, root)) != len(after) {
		t.Errorf("the second run changed or added a tape (%d vs %d bytes, %d tapes)", len(b1), len(b2), len(tapes(t, root)))
	}
	// The text output does not count the cut as the writer of the lines.
	again, _ := runTraceBinary(t, root, show, []string{"SRWR_LANG="})
	if !strings.Contains(again, "one に出力を足すため") || strings.Contains(again, id+" #") {
		t.Errorf("the cut counts as a source:\n%s", again)
	}
}

// buildState replays events of one tape and returns the text of each file.
func buildState(events []tape.Event) map[string]string {
	st := tape.Build(events)
	out := map[string]string{}
	for name, f := range st.Files {
		out[name] = f.Text
	}
	return out
}
