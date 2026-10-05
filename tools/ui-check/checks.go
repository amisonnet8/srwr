package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// A check is an item of the UI that is decided by a machine. Each is decided by the tests it names; it passes when every one of them ran and passed. A name that no test has
// fails the check (a renamed test must not silently stop checking).
type check struct {
	Name  string
	What  string
	Tests []testRef
}

// testRef names tests. Kind go: Where is "<package dir>:<test name>". Kind node: Where is a part of the test's name (the
// extension's tests are named by a sentence). Kind vim: Where is a file under vim/test/.
type testRef struct{ Kind, Where string }

func goTest(pkg, name string) testRef { return testRef{"go", pkg + ":" + name} }
func nodeTest(part string) testRef    { return testRef{"node", part} }
func vimTest(file string) testRef     { return testRef{"vim", file} }

var checks = []check{
	{"起動と一覧", "項目の文言・並び・件数", []testRef{
		nodeTest("tapes/list: the three fixed tapes"), nodeTest("quick pick: placeholder and items"), nodeTest("the list: numbered from 1"), vimTest("test_list.vim")}},
	{"look・edit の表示", "理由の行の位置・範囲・色の値", []testRef{
		nodeTest("a look frame: why row in blue"), nodeTest("an edit frame is orange"), vimTest("test_view.vim"), vimTest("test_hl.vim")}},
	{"行番号", "自前の番号の値・標準に戻る", []testRef{
		nodeTest("a normal frame after a diff frame stated"), nodeTest("the line numbers are as wide as"), vimTest("test_pure.vim")}},
	{"理由なしのコマ", "理由の行が無い", []testRef{nodeTest("a frame without why"), vimTest("test_view.vim")}},
	{"長い理由の折り返し", "行数・字下げ・全文・幅ごとの違い", []testRef{
		nodeTest("wrapWhy"), goTest("tools/ui-check", "TestLongWhyTapeIsMadeByTheRealMCP"), goTest("tools/ui-check", "TestVerifyLongWhy")}},
	{"失敗のコマと、表示する種類", "絞り込み（kinds）・番号の振り直し・隠した数・ライブの新着・赤い行・キー（ts tr te tf）・漏斗ボタン", []testRef{
		goTest("internal/viewserver", "TestTapeOpenKinds"), goTest("internal/viewserver", "TestFrameStateWithKinds"), goTest("internal/viewserver", "TestLiveKinds"),
		goTest("tools/ui-check", "TestFailureTapeIsMadeByTheRealMCP"), nodeTest("the funnel button"), nodeTest("a failure frame"), vimTest("test_kinds.vim")}},
	{"差分のコマ", "左右の塗り・見出し・最初の変更行が上から3割・行き来しても増えない", []testRef{
		nodeTest("a diff frame: two editors"), nodeTest("a final frame: headings"), nodeTest("going back and forth over diff frames never piles up editors"), vimTest("test_diff.vim")}},
	{"操作一覧", "行の文字列・丸の色・今の行", []testRef{nodeTest("the list: numbered from 1, kinds, dots"), vimTest("test_list.vim")}},
	{"下のバー", "文字列・薄い側の判定", []testRef{nodeTest("stepping: always a back and a forward button"), vimTest("test_decisions.vim")}},
	{"コマ送りの操作", "連打・端での操作・キーの割り当ては <buffer> だけ", []testRef{nodeTest("quick successive steps end on the last one asked for"), vimTest("test_keys.vim")}},
	{"ライブ", "追っている・遅れている・新着・最新へ戻る・閉じる", []testRef{
		nodeTest("following: each new frame is shown at once"), nodeTest("stepping back stops following"), nodeTest("clicking an older frame also stops following"), vimTest("test_live.vim")}},
	{"ダーク・ライト", "色の定義の値", []testRef{vimTest("test_hl.vim"), goTest("tools/ui-check", "TestVimInitIsThePlainColorsOfTheImages")}},
	{"後始末", "タブ・ウィンドウ・バッファの数が開く前に戻る", []testRef{
		nodeTest("closing: the list and the bar go away"), nodeTest("opening another tape replaces the first"), vimTest("test_diff.vim"), vimTest("test_view.vim")}},
	{"失敗の案内", "文言", []testRef{nodeTest("the binary is missing"), nodeTest("any other error is shown"), vimTest("test_errors.vim")}},
	{"既知の不具合（2件）", "範囲が下端で隠れない・LIVE に戻るが切れない（回帰テスト）", []testRef{
		goTest("vim", "TestRangeIsNeverHiddenUnderTheWhyRows"), vimTest("test_decisions.vim")}},
}

// CheckResult is the outcome of one check.
type CheckResult struct {
	Name  string   `json:"name"`
	What  string   `json:"what"`
	OK    bool     `json:"ok"`
	Notes []string `json:"notes,omitempty"` // why it failed
}

// outcome of one test: pass, fail or missing (no test of that name ran).
type outcome struct {
	ran, failed bool
	output      string
}

// runChecks runs the tests the checks name and decides every check. A runner that cannot run fails the checks that need it.
func runChecks(root string, log func(string)) []CheckResult {
	results := map[string]*outcome{}
	var goNames = map[string][]string{}
	var vimFiles []string
	for _, c := range checks {
		for _, t := range c.Tests {
			switch t.Kind {
			case "go":
				pkg, name, _ := strings.Cut(t.Where, ":")
				if !slices.Contains(goNames[pkg], name) {
					goNames[pkg] = append(goNames[pkg], name)
				}
			case "vim":
				if !slices.Contains(vimFiles, t.Where) {
					vimFiles = append(vimFiles, t.Where)
				}
			}
		}
	}
	log("自動の検証：Go のテスト")
	runGoTests(root, goNames, results)
	log("自動の検証：拡張のテスト")
	nodeNames := runNodeTests(root)
	log("自動の検証：Vim のテスト")
	runVimTests(root, vimFiles, results)

	var out []CheckResult
	for _, c := range checks {
		out = append(out, decide(c, nodeNames, results))
	}
	return out
}

// decide says whether a check passed: every test it names ran and passed.
func decide(c check, nodeNames nodeTests, results map[string]*outcome) CheckResult {
	r := CheckResult{Name: c.Name, What: c.What, OK: true}
	for _, t := range c.Tests {
		var o *outcome
		switch t.Kind {
		case "node":
			o = matchNode(nodeNames, t.Where)
		default:
			o = results[t.Kind+":"+t.Where]
		}
		switch {
		case o == nil || !o.ran:
			r.OK = false
			r.Notes = append(r.Notes, fmt.Sprintf("%s のテスト %q が動かなかった（名前が変わった？）", t.Kind, t.Where))
		case o.failed:
			r.OK = false
			r.Notes = append(r.Notes, fmt.Sprintf("%s のテスト %q が落ちた：%s", t.Kind, t.Where, o.output))
		}
	}
	return r
}

func capture(dir string, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...) //nolint:gosec // fixed tools: go, node, the vim named by VIM_BIN
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	return cmd.CombinedOutput()
}

func runGoTests(root string, names map[string][]string, results map[string]*outcome) {
	var pkgs []string
	for p := range names {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	for _, pkg := range pkgs {
		pattern := "^(" + strings.Join(names[pkg], "|") + ")$"
		out, _ := capture(root, []string{"SRWR_SCREEN_TEST=1"}, "go", "test", "-json", "-count=1", "-run", pattern, "./"+pkg)
		failOutput := map[string]*strings.Builder{}
		sc := bufio.NewScanner(bytes.NewReader(out))
		sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
		for sc.Scan() {
			var e struct{ Action, Test, Output string }
			if json.Unmarshal(sc.Bytes(), &e) != nil || e.Test == "" || strings.Contains(e.Test, "/") {
				continue
			}
			key := "go:" + pkg + ":" + e.Test
			o := results[key]
			if o == nil {
				o = &outcome{}
				results[key] = o
			}
			switch e.Action {
			case "output":
				b := failOutput[key]
				if b == nil {
					b = &strings.Builder{}
					failOutput[key] = b
				}
				b.WriteString(e.Output)
			case "pass":
				o.ran = true
			case "fail":
				o.ran, o.failed = true, true
				if b := failOutput[key]; b != nil {
					o.output = lastLines(b.String(), 6)
				}
			case "skip":
				o.ran, o.failed = true, true
				o.output = "skipped"
			}
		}
	}
}

// nodeTests are the results of the extension's tests, by the name of the test.
type nodeTests map[string]bool

var tapLine = regexp.MustCompile(`^(not ok|ok) \d+ - (.*?)(?: # .*)?$`)

func runNodeTests(root string) nodeTests {
	out, _ := capture(filepath.Join(root, "extension"), nil, "node", "--test", "--test-reporter=tap", "--require", "./out/test/setup.js", "out/test/*.test.js")
	res := nodeTests{}
	for _, l := range strings.Split(string(out), "\n") {
		if m := tapLine.FindStringSubmatch(l); m != nil { // only the top-level lines (no indentation)
			res[m[2]] = m[1] == "ok"
		}
	}
	return res
}

// matchNode decides a node ref: every test whose name contains the part must have passed, and there must be one.
func matchNode(res nodeTests, part string) *outcome {
	o := &outcome{}
	for name, ok := range res {
		if strings.Contains(name, part) {
			o.ran = true
			if !ok {
				o.failed = true
				o.output = name
			}
		}
	}
	return o
}

func runVimTests(root string, files []string, results map[string]*outcome) {
	vim := os.Getenv("VIM_BIN")
	if vim == "" {
		vim = "vim"
	}
	for _, f := range files {
		o := &outcome{ran: true}
		results["vim:"+f] = o
		out, err := capture(root, nil, vim, "-Nu", "NONE", "-i", "NONE", "-Es", "-V1", "-S", filepath.Join("vim", "test", f), "-c", "cquit 2")
		if err != nil {
			o.failed = true
			o.output = lastLines(string(out), 6)
		}
	}
}

// lastLines is the end of the output of a test for a message: the last n lines, cut to 240 characters.
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	out := []rune(strings.Join(lines, " / "))
	if len(out) > 240 {
		return string(out[:240]) + "…"
	}
	return string(out)
}
