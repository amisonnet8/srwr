package docs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/amisonnet8/srwr/internal/cli"
	"github.com/amisonnet8/srwr/internal/core"
	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/tape"
)

// The example in docs/examples/workflow.md is a round from `srwr init` to the tidying of tapes. Every command in it is run for real,
// in a workspace made here, and the output the document shows must be what comes out. What changes from run to run (the last four
// characters of a tape ID, the sizes, the path of the workspace) is hidden first. In dump mode (WORKFLOW_DUMP=1) the real
// output is printed instead, for writing the document.

const workflowMain = `package main

import "fmt"

func main() {
	fmt.Println(greet(""))
}

func greet(name string) string {
	return "hello " + name
}
`

var (
	workflowIDRe   = regexp.MustCompile(`(\d{8}-\d{4})-[0-9a-z]{4}`)
	workflowSizeRe = regexp.MustCompile(`\d+\.\d KB`)
	workflowNoteRe = regexp.MustCompile(`(?m)^(Note: srwr is not on your PATH|注意：srwr が PATH にありません).*\n?`)
)

type workflowRun struct {
	t      *testing.T
	root   string
	japan  bool
	clock  time.Time
	tapeID []string // the IDs of the tapes the AI made, oldest first
}

func (w *workflowRun) now() time.Time { w.clock = w.clock.Add(time.Second); return w.clock }

func (w *workflowRun) why(en, ja string) string {
	if w.japan {
		return ja
	}
	return en
}

func (w *workflowRun) git(args ...string) {
	w.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=demo", "-c", "user.email=demo@example.com"}, args...)...) //nolint:gosec // git, in a temporary directory
	cmd.Dir = w.root
	if out, err := cmd.CombinedOutput(); err != nil {
		w.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (w *workflowRun) write(name, text string) {
	w.t.Helper()
	if err := os.WriteFile(filepath.Join(w.root, name), []byte(text), 0o600); err != nil {
		w.t.Fatal(err)
	}
}

// ai makes the calls the AI makes in a session that starts at the given time.
func (w *workflowRun) ai(start time.Time, calls func(c *core.Core)) {
	w.t.Helper()
	w.clock = start
	ws, err := session.Open(w.root, session.Options{Version: "v0.1.3", Now: w.now})
	if err != nil {
		w.t.Fatal(err)
	}
	ws.SetAuthor(tape.Author{Kind: "ai", Name: "claude"})
	calls(&core.Core{WS: ws})
	id, ok := ws.Current()
	if !ok {
		w.t.Fatal("the AI made no tape")
	}
	w.tapeID = append(w.tapeID, id)
}

func (w *workflowRun) edit(c *core.Core, file string, start, end int, text, whyEN, whyJA string) {
	w.t.Helper()
	sel, cerr := c.Select(core.SelectInput{File: file, StartLine: start, EndLine: end, Why: w.why("Look at "+file, file+" を見る")})
	if cerr != nil {
		w.t.Fatal(cerr)
	}
	if _, cerr := c.Replace(core.ReplaceInput{Selection: sel.Selection, NewText: text, Why: w.why(whyEN, whyJA)}); cerr != nil {
		w.t.Fatal(cerr)
	}
}

// before returns what happens in the workspace before the command of block i is typed.
func (w *workflowRun) before(i int) {
	switch i {
	case 0: // a project under git
		w.write("main.go", workflowMain)
		w.write("README.md", "# demo\n")
		w.git("init", "-q")
		w.git("add", "-A")
		w.git("commit", "-q", "-m", "first")
	case 1: // srwr init is done and committed; the AI works
		w.git("add", "-A")
		w.git("commit", "-q", "-m", "Set up srwr")
		w.ai(time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC), func(c *core.Core) {
			w.edit(c, "main.go", 9, 11, "func greet(name string) string {\n\tif name == \"\" {\n\t\tname = \"stranger\"\n\t}\n\treturn \"hello \" + name\n}",
				"An empty name printed \"hello \" with nothing after it, so greet a stranger instead",
				"名前が空だと \"hello \" だけが出るので、知らない人に挨拶する形にする")
		})
	case 2: // somebody edits a file by hand, outside srwr
		w.write("README.md", "# demo\n\nRun it with `go run .`\n")
	case 4: // the next session
		w.ai(time.Date(2026, 10, 4, 13, 0, 0, 0, time.UTC), func(c *core.Core) {
			w.edit(c, "main.go", 6, 6, "\tfmt.Println(greet(\"Ada\"))",
				"greet handles the empty name now, so main can greet a real one",
				"空の名前は greet が扱うようになったので、main は本物の名前で呼ぶ")
		})
	}
}

// normalize hides what changes from run to run, and puts the workspace at /work.
func (w *workflowRun) normalize(s string) string {
	s = strings.ReplaceAll(s, w.root, "/work")
	s = strings.ReplaceAll(s, filepath.ToSlash(w.root), "/work")
	s = strings.ReplaceAll(s, "\\", "/")
	s = workflowNoteRe.ReplaceAllString(s, "")
	s = workflowIDRe.ReplaceAllString(s, "$1-xxxx")
	s = workflowSizeRe.ReplaceAllString(s, "N KB")
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		lines = append(lines, strings.TrimRight(l, " \t"))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// run types the command line of a block (without the leading "$ ") and returns what it printed and its exit code.
func (w *workflowRun) run(line string, docIDs []string) (string, int) {
	fields := strings.Fields(strings.TrimPrefix(line, "srwr "))
	for i, f := range fields { // a tape ID in the document stands for the tape of the same place in the run
		for k, id := range docIDs {
			if workflowIDRe.MatchString(f) && workflowIDRe.FindString(f) == workflowIDRe.FindString(id) && k < len(w.tapeID) {
				fields[i] = w.tapeID[k]
			}
		}
	}
	at := w.clock.Add(10 * time.Minute) // ten minutes after the last thing the AI did: the session is still the current one
	old := cli.Now
	cli.Now = func() time.Time { return at }
	defer func() { cli.Now = old }()
	var out, errOut strings.Builder
	code := cli.Run(append(fields, "--root", w.root), nil, &out, &errOut)
	return out.String() + errOut.String(), code
}

func newWorkflowRun(t *testing.T, japan bool) *workflowRun {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	lang := ""
	if japan {
		lang = "ja"
	}
	t.Setenv("SRWR_LANG", lang)
	old := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = old })
	return &workflowRun{t: t, root: root, japan: japan}
}

func TestWorkflowExample(t *testing.T) {
	for _, japan := range []bool{false, true} {
		name, suffix := "workflow", ""
		if japan {
			suffix = "_ja"
		}
		t.Run(name+suffix, func(t *testing.T) {
			path := filepath.Join("..", "..", "docs", "examples", name+suffix+".md")
			w := newWorkflowRun(t, japan)
			if os.Getenv("WORKFLOW_DUMP") != "" {
				for i := 0; i < 7; i++ {
					w.before(i)
					cmds := [][]string{{"srwr init"}, {"srwr tapes"}, {"srwr tapes check"}, {"srwr tapes new"}, {"srwr tapes"}, {"srwr tapes path ID"}, {"srwr tapes prune --keep 1"}}
					line := cmds[i][0]
					if strings.HasSuffix(line, " ID") {
						line = strings.TrimSuffix(line, " ID") + " " + w.tapeID[0]
					}
					out, code := w.run(line, nil)
					fmt.Printf("=== %d $ %s (exit %d)\n%s\n", i, line, code, w.normalize(out))
				}
				return
			}
			blocks := codeBlocks(t, path, "console")
			if len(blocks) != 7 {
				t.Fatalf("the document has %d console blocks, want 7", len(blocks))
			}
			var docIDs []string
			for _, b := range blocks {
				for _, l := range b {
					docIDs = append(docIDs, workflowIDRe.FindAllString(l, -1)...)
				}
			}
			docIDs = dedupe(docIDs)
			for i, b := range blocks {
				if len(b) == 0 || !strings.HasPrefix(b[0], "$ srwr ") {
					t.Fatalf("block %d does not start with a command: %q", i+1, b)
				}
				w.before(i)
				got, _ := w.run(strings.TrimPrefix(b[0], "$ "), docIDs)
				want := strings.Join(b[1:], "\n")
				if w.normalize(got) != w.normalize(want) {
					t.Errorf("block %d (%s) differs.\nthe command printed:\n%s\n\nthe document shows:\n%s", i+1, b[0], w.normalize(got), w.normalize(want))
				}
			}
		})
	}
}

func dedupe(ids []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
