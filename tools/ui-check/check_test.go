package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/uicheck"
)

// The items of handoff/checklist/CHECKLIST.md 2章 that a machine decides. If an item is dropped from the table, nobody checks it.
var checklistItems = []string{"起動と一覧", "select・replace の表示", "行番号", "理由なしのコマ", "長い理由の折り返し", "差分のコマ", "操作一覧",
	"下のバー", "コマ送りの操作", "ライブ", "ダーク・ライト", "後始末", "失敗の案内", "既知の不具合（2件）"}

func TestEveryChecklistItemHasTestsThatExist(t *testing.T) {
	var names []string
	for _, c := range checks {
		names = append(names, c.Name)
		if len(c.Tests) == 0 {
			t.Errorf("%s: no test", c.Name)
		}
		for _, ref := range c.Tests {
			switch ref.Kind {
			case "vim":
				if _, err := os.Stat(filepath.Join("..", "..", "vim", "test", ref.Where)); err != nil {
					t.Errorf("%s: %v", c.Name, err)
				}
			case "go":
				pkg, name, _ := strings.Cut(ref.Where, ":")
				if !grepDir(t, filepath.Join("..", "..", pkg), "_test.go", regexp.MustCompile(`func `+name+`\(`)) {
					t.Errorf("%s: no Go test %s in %s", c.Name, name, pkg)
				}
			case "node":
				if !grepDir(t, filepath.Join("..", "..", "extension", "test"), ".test.ts", regexp.MustCompile(`test\("[^"]*`+regexp.QuoteMeta(ref.Where))) {
					t.Errorf("%s: no extension test with %q in its name", c.Name, ref.Where)
				}
			default:
				t.Errorf("%s: unknown kind %q", c.Name, ref.Kind)
			}
		}
	}
	if strings.Join(names, ",") != strings.Join(checklistItems, ",") {
		t.Errorf("the items are %v, want the items of CHECKLIST 2章 %v", names, checklistItems)
	}
}

func grepDir(t *testing.T, dir, suffix string, re *regexp.Regexp) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), suffix) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name())) //nolint:gosec // the repository's own test directory
		if err != nil {
			t.Fatal(err)
		}
		if re.Match(b) {
			return true
		}
	}
	return false
}

func TestMatchNodeNeedsATestThatRanAndPassed(t *testing.T) {
	res := nodeTests{"a replace frame is orange": true, "a select frame: why": false}
	if o := matchNode(res, "replace frame"); !o.ran || o.failed {
		t.Errorf("a passing test: %+v", o)
	}
	if o := matchNode(res, "select frame"); !o.ran || !o.failed {
		t.Errorf("a failing test: %+v", o)
	}
	if o := matchNode(res, "no such test"); o.ran {
		t.Errorf("a name no test has must not count as run: %+v", o)
	}
}

func okChecks() []CheckResult {
	return []CheckResult{{Name: "行番号", What: "x", OK: true}}
}

func TestPageSaysWhatHappened(t *testing.T) {
	diff := CaptureResult{Name: "replay-why-basic dark", Status: statusDiff, Frames: []FrameInfo{{Index: 5, Label: "5", Diffs: []string{"background: 3 cells differ"}, Want: "<svg>a</svg>", Got: "<svg>b</svg>"}}}
	for name, tc := range map[string]struct {
		rep  Report
		want []string
		not  []string
	}{
		"clean":  {Report{Checks: okChecks(), Decision: Decision{State: "pending"}}, []string{"違いなし", "自動の検証 1/1 通過", "変更が無いので、確認は要りません"}, []string{"基準と違うコマ 1"}},
		"a diff": {Report{Checks: okChecks(), Vim: []CaptureResult{diff}}, []string{"基準と違うコマ 1", "replay-why-basic dark", "5 コマ目", "background: 3 cells differ", "<svg>a</svg>", "<svg>b</svg>", "上の画像「Vim・replay-why-basic dark」を見る"}, []string{"違いなし</div>"}},
		"a failed check": {Report{Checks: []CheckResult{{Name: "差分のコマ", What: "x", OK: false, Notes: []string{"vim のテスト \"test_diff.vim\" が落ちた"}}}},
			[]string{"自動の検証が 1 件失敗", "差分のコマ", "test_diff.vim"}, []string{"違いなし</div>"}},
		"a new one": {Report{Checks: okChecks(), Vim: []CaptureResult{{Name: "replay-long-why dark", Status: statusNew, Frames: []FrameInfo{{Index: 1, Label: "1", Got: "<svg>n</svg>"}}}}},
			[]string{"新しいコマ", "基準がありません", "<svg>n</svg>"}, []string{"違いなし</div>"}},
		"the extension, new": {Report{Checks: okChecks(), VSCode: []CaptureResult{{Name: "long_why", Status: statusNew, Frames: []FrameInfo{{Index: 1, Label: "1"}, {Index: 2, Label: "2"}}}}},
			[]string{"2 コマ。画像はありません", "<code>qsoku ui-open vscode long-why</code> と打って Enter", "左端のカセットのアイコンを押し", "「テープを開く」", "「進む」を押して"}, []string{"<svg", "…"}},
		"the extension, differs": {Report{Checks: okChecks(), VSCode: []CaptureResult{{Name: "all_basic", Status: statusDiff, Frames: []FrameInfo{{Index: 5, Label: "frame 4", Diffs: []string{"editor 1 decorations: baseline only line 36 #b45f06; now only line 36 #c05f06"}}}}}},
			[]string{"VSCode（拡張）", "5 コマ目", "#c05f06"}, []string{"<svg", "簡素な図"}},
		"an error":  {Report{Checks: okChecks(), VSCode: []CaptureResult{{Name: "all_basic", Status: statusError, Error: "no capture"}}}, []string{"取れなかった", "no capture"}, []string{"違いなし</div>"}},
		"a problem": {Report{Checks: okChecks(), Problems: []string{"長い理由（x）：one row"}}, []string{"見つかった問題", "one row"}, []string{"違いなし</div>"}},
	} {
		page := renderPage(&tc.rep)
		for _, w := range tc.want {
			if !strings.Contains(page, w) {
				t.Errorf("%s: the page lacks %q", name, w)
			}
		}
		for _, n := range tc.not {
			if strings.Contains(page, n) {
				t.Errorf("%s: the page has %q", name, n)
			}
		}
	}
}

// A workspace as qsoku ui-check leaves it: a baseline in the repository, and a result that differs from it.
func resultRoot(t *testing.T, state string, checkOK bool) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := `{"cols":4,"rows":2,"normal":"#1e1e1e","frames":[
{"label":"1","skip":[1],"text":["old",""],"bg":[],"fg":[]}
]}`
	now := `{"cols":4,"rows":2,"normal":"#1e1e1e","frames":[
{"label":"1","text":["new",""],"bg":[],"fg":[]}
]}`
	write("vim/test/baseline/a_dark.json", old)
	write("vim/test/baseline/README.md", "# baseline\n")
	write("extension/test/baseline/all_x.json", `[{"label":"old"}]`)
	write("extension/test/baseline/README.md", "# baseline\n")
	rep := Report{Dir: "20261003-000000", Version: "v1", Decision: Decision{State: state},
		Checks: []CheckResult{{Name: "行番号", OK: checkOK}},
		Vim:    []CaptureResult{{Name: "a dark", Status: statusDiff, File: "vim/a_dark.json", Target: "vim/test/baseline/a_dark.json"}, {Name: "b dark", Status: statusNew, File: "vim/b_dark.json", Target: "vim/test/baseline/b_dark.json"}, {Name: "same dark", Status: statusSame}},
		VSCode: []CaptureResult{{Name: "all_x", Status: statusDiff, File: "vscode/all_x.json", Target: "extension/test/baseline/all_x.json"}}}
	b, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"ui-check-result/latest", "ui-check-result/20261003-000000"} {
		write(dir+"/result.json", string(b))
		write(dir+"/vim/a_dark.json", now)
		write(dir+"/vim/b_dark.json", now)
		write(dir+"/vscode/all_x.json", `[{"label":"new"}]`)
	}
	return root
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p) //nolint:gosec // a temporary directory
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAcceptOKReplacesTheBaselineAndKeepsTheSkippedRows(t *testing.T) {
	root := resultRoot(t, "pending", true)
	var out strings.Builder
	if err := acceptResult(root, true, "", &out); err != nil {
		t.Fatal(err)
	}
	a := readFile(t, filepath.Join(root, "vim/test/baseline/a_dark.json"))
	if !strings.Contains(a, `"new"`) || strings.Contains(a, `"old"`) {
		t.Errorf("the baseline of a was not replaced: %s", a)
	}
	if !strings.Contains(a, `"skip":[1]`) {
		t.Errorf("the skipped row of the old baseline was lost: %s", a)
	}
	if b := readFile(t, filepath.Join(root, "vim/test/baseline/b_dark.json")); !strings.Contains(b, `"new"`) {
		t.Errorf("a new capture did not become a baseline: %s", b)
	}
	if v := readFile(t, filepath.Join(root, "extension/test/baseline/all_x.json")); !strings.Contains(v, "new") {
		t.Errorf("the baseline of the extension was not replaced: %s", v)
	}
	for _, r := range []string{"vim/test/baseline/README.md", "extension/test/baseline/README.md"} {
		if !strings.Contains(readFile(t, filepath.Join(root, r)), "ui-accept") {
			t.Errorf("%s has no note of the change", r)
		}
	}
	var rep Report
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(root, "ui-check-result/latest/result.json"))), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Decision.State != "ok" || rep.Decision.At == "" {
		t.Errorf("decision = %+v", rep.Decision)
	}
	if err := acceptResult(root, true, "", &out); err == nil {
		t.Error("a result decided once was decided again")
	}
}

func TestAcceptNGKeepsTheBaseline(t *testing.T) {
	root := resultRoot(t, "pending", true)
	var out strings.Builder
	if err := acceptResult(root, false, "色が読めない", &out); err != nil {
		t.Fatal(err)
	}
	if a := readFile(t, filepath.Join(root, "vim/test/baseline/a_dark.json")); !strings.Contains(a, `"old"`) {
		t.Errorf("NG changed the baseline: %s", a)
	}
	if _, err := os.Stat(filepath.Join(root, "vim/test/baseline/b_dark.json")); err == nil {
		t.Error("NG made a baseline")
	}
	var rep Report
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(root, "ui-check-result/latest/result.json"))), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Decision.State != "ng" || rep.Decision.Note != "色が読めない" {
		t.Errorf("decision = %+v", rep.Decision)
	}
}

func TestAcceptRefusesWhileAChecksFails(t *testing.T) {
	root := resultRoot(t, "pending", false)
	var out strings.Builder
	if err := acceptResult(root, true, "", &out); err == nil {
		t.Fatal("OK was recorded although a check failed")
	}
	if a := readFile(t, filepath.Join(root, "vim/test/baseline/a_dark.json")); !strings.Contains(a, `"old"`) {
		t.Errorf("the baseline changed: %s", a)
	}
}

// Break the Vim client on purpose: qsoku ui-check must say which screen and what differs. (Needs a real Vim and bin/srwr.)
func TestBreakingTheVimClientIsFoundAndNamed(t *testing.T) {
	if os.Getenv("SRWR_SCREEN_TEST") == "" {
		t.Skip("set SRWR_SCREEN_TEST=1 (qsoku vim-test does)")
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(repo, "bin", "srwr")
	if _, err := os.Stat(bin); err != nil {
		t.Skip("bin/srwr is not built (qsoku bin)")
	}
	root := t.TempDir()
	for _, d := range [][2]string{{"vim", "vim"}, {"extension/test/fixtures/ui-check", "extension/test/fixtures/ui-check"}} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, d[1])), 0o750); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command("cp", "-r", filepath.Join(repo, d[0]), filepath.Join(root, d[1])).CombinedOutput(); err != nil { //nolint:gosec // paths made by this test
			t.Fatalf("cp: %v\n%s", err, out)
		}
	}
	hl := filepath.Join(root, "vim", "autoload", "srwr", "hl.vim")
	src := readFile(t, hl)
	if !strings.Contains(src, "guibg: '#0b61a4'") {
		t.Fatal("the select color is not where this test breaks it")
	}
	if err := os.WriteFile(hl, []byte(strings.Replace(src, "guibg: '#0b61a4'", "guibg: '#a40b61'", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "vim"), 0o750); err != nil {
		t.Fatal(err)
	}
	var sc screenRun
	for _, s := range screenRuns {
		if s.Name == "replay-why-basic" {
			sc = s
		}
	}
	rep := &Report{}
	res := captureVimOne(root, bin, dir, "", sc, "dark", rep)
	if res.Status != statusDiff {
		t.Fatalf("status = %s (%s), want diff", res.Status, res.Error)
	}
	first := res.Frames[0]
	if first.Index != 1 || !strings.Contains(strings.Join(first.Diffs, "\n"), "background") || !strings.Contains(strings.Join(first.Diffs, "\n"), "#a40b61") {
		t.Errorf("the first differing frame is %d: %v", first.Index, first.Diffs)
	}
	if first.Want == "" || first.Got == "" {
		t.Error("no pictures for the page")
	}
	page := renderPage(&Report{Checks: okChecks(), Vim: []CaptureResult{res}})
	if !strings.Contains(page, "replay-why-basic dark") || !strings.Contains(page, "stroke=\"#ff2d2d\"") {
		t.Error("the page does not name the screen or mark the cells")
	}
	_ = uicheck.Baseline{}
}

func TestDecideNeedsEveryTestToHaveRunAndPassed(t *testing.T) {
	c := check{Name: "x", Tests: []testRef{goTest("a", "TestA"), nodeTest("part"), vimTest("t.vim")}}
	pass := map[string]*outcome{"go:a:TestA": {ran: true}, "vim:t.vim": {ran: true}}
	nodes := nodeTests{"a part of it": true}
	if r := decide(c, nodes, pass); !r.OK || len(r.Notes) != 0 {
		t.Fatalf("all passed: %+v", r)
	}
	for name, tc := range map[string]struct {
		res   map[string]*outcome
		nodes nodeTests
		note  string
	}{
		"go failed":      {map[string]*outcome{"go:a:TestA": {ran: true, failed: true, output: "boom"}, "vim:t.vim": {ran: true}}, nodes, "boom"},
		"go missing":     {map[string]*outcome{"vim:t.vim": {ran: true}}, nodes, "TestA"},
		"vim failed":     {map[string]*outcome{"go:a:TestA": {ran: true}, "vim:t.vim": {ran: true, failed: true}}, nodes, "t.vim"},
		"node failed":    {pass, nodeTests{"a part of it": false}, "part"},
		"node not found": {pass, nodeTests{"other": true}, "part"},
		"go did not run": {map[string]*outcome{"go:a:TestA": {}, "vim:t.vim": {ran: true}}, nodes, "TestA"},
	} {
		r := decide(c, tc.nodes, tc.res)
		if r.OK || !strings.Contains(strings.Join(r.Notes, "\n"), tc.note) {
			t.Errorf("%s: %+v", name, r)
		}
	}
}

func TestCleanAndCounts(t *testing.T) {
	same := CaptureResult{Name: "s", Status: statusSame}
	diff := CaptureResult{Name: "d", Status: statusDiff, Frames: []FrameInfo{{Index: 1}, {Index: 2}}}
	fresh := CaptureResult{Name: "n", Status: statusNew}
	broken := CaptureResult{Name: "e", Status: statusError}
	for name, tc := range map[string]struct {
		rep               Report
		clean             bool
		diffs, newScreens int
	}{
		"nothing":           {Report{Checks: okChecks(), Vim: []CaptureResult{same}}, true, 0, 0},
		"new is not a diff": {Report{Checks: okChecks(), Vim: []CaptureResult{fresh}}, true, 0, 1},
		"a diff":            {Report{Checks: okChecks(), VSCode: []CaptureResult{diff}}, false, 2, 0},
		"an error":          {Report{Checks: okChecks(), Vim: []CaptureResult{broken}}, false, 0, 0},
		"a failed check":    {Report{Checks: []CheckResult{{OK: false}}}, false, 0, 0},
		"a problem":         {Report{Checks: okChecks(), Problems: []string{"x"}}, false, 0, 0},
	} {
		if got := tc.rep.Clean(); got != tc.clean {
			t.Errorf("%s: Clean = %v", name, got)
		}
		if got := tc.rep.DiffCount(); got != tc.diffs {
			t.Errorf("%s: DiffCount = %d", name, got)
		}
		if got := tc.rep.NewCount(); got != tc.newScreens {
			t.Errorf("%s: NewCount = %d", name, got)
		}
	}
}

func TestLookStepsAreExactAndNeedNoThinking(t *testing.T) {
	rep := &Report{Checks: okChecks(),
		Vim:    []CaptureResult{{Name: "replay-long-why dark", Status: statusNew}, {Name: "replay-external light", Status: statusDiff}, {Name: "live-basic dark", Status: statusSame}},
		VSCode: []CaptureResult{{Name: "long_why", Status: statusNew, Frames: []FrameInfo{{Index: 1}}}, {Name: "all_ext", Status: statusDiff, Frames: []FrameInfo{{Index: 1}}}}}
	steps := lookSteps(rep)
	if len(steps) != 4 {
		t.Fatalf("%d steps: %q", len(steps), steps)
	}
	all := strings.Join(steps, "\n")
	for _, want := range []string{"Vim・replay-long-why dark", "折り返され", "Vim・replay-external light", "赤枠", "<code>qsoku ui-open vscode long-why</code> と打って Enter を押す", "<code>qsoku ui-open vscode external</code> と打って Enter を押す", "「テープを開く」"} {
		if !strings.Contains(all, want) {
			t.Errorf("the steps lack %q:\n%s", want, all)
		}
	}
	if strings.Contains(all, "…") || strings.Contains(all, "live-basic") {
		t.Errorf("a step has an ellipsis or is about a screen that did not change:\n%s", all)
	}
	if got := lookSteps(&Report{Checks: okChecks()}); len(got) != 0 {
		t.Errorf("steps for a run without changes: %q", got)
	}
}

func TestAddLongWhyPutsTheTapeAndFileInTheWorkspace(t *testing.T) {
	bin, err := filepath.Abs("../../bin/srwr")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Skip("bin/srwr is not built (qsoku bin)")
	}
	ws := t.TempDir()
	if err := prepareWorkspace(fixture, ws, bin, false); err != nil {
		t.Fatal(err)
	}
	if err := addLongWhy(bin, ws); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"a.go", filepath.Join(".srwr", "tapes", longWhyTape+".tape.jsonl"), filepath.Join(".srwr", "tapes", tapes["why-basic"].ID+".tape.jsonl")} {
		if _, err := os.Stat(filepath.Join(ws, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}
