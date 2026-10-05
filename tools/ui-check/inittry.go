package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/amisonnet8/srwr/internal/tape"
)

// init-try is the human check of R9: a workspace that already has settings of its own gets `srwr init`, Claude Code works in it
// in the workspace init prepared (the strict mode: Edit is forbidden, so look and edit do the editing), and this command checks
// the files, the second run of init and the tape by itself. The person starts it and reads the marks on the page.

const initTryTask = `# 作業

このディレクトリの Go のテストが2つ落ちています。原因を調べて直してください。

1. まず、落ちているテストと、関係するファイルを、Read・Grep・Bash（cat、sed -n、grep -n）で調べる
2. text.go の Truncate は、srwr の look と edit（MCP ツール）で直す。why は日本語で書く
3. pad.go の PadLeft は、まず Edit で直そうとする。Edit が使えなければ、srwr の look と edit で直す
4. 最後に go test ./... を実行して、通ることを確かめる
`

// initTryMCP and initTrySettings are the files the workspace had before init: a server and settings that are not srwr's.
const initTryMCP = `{"mcpServers":{"other":{"command":"true"}}}`
const initTrySettings = `{"env":{"SRWR_TRY":"1"},"permissions":{"deny":["Bash(rm:*)"]}}`

func prepareInitTry(dst string) error {
	if err := os.RemoveAll(dst); err != nil { //nolint:gosec // the workspace of this tool, under the temporary directory
		return err
	}
	files := map[string]string{"TASK.md": initTryTask, ".mcp.json": initTryMCP, ".claude/settings.json": initTrySettings}
	for name, text := range hookTryFiles {
		if name != "TASK.md" {
			files[name] = text
		}
	}
	for name, text := range files {
		p := filepath.Join(dst, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil { //nolint:gosec // inside the workspace of this tool
			return err
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil { //nolint:gosec // inside the workspace of this tool
			return err
		}
	}
	return nil
}

// initTryAllow is hookTryAllow without Edit: the allow list of the command line must not undo what init forbids.
func initTryAllow() []string {
	var l []string
	for _, a := range hookTryAllow {
		if a != "Edit" {
			l = append(l, a)
		}
	}
	return l
}

func initTryClaudeArgs() []string {
	return []string{"-p", "Do what TASK.md says", "--allowedTools", strings.Join(initTryAllow(), ","),
		"--mcp-config", ".mcp.json"}
}

// runIn runs a command in dir with binDir first on PATH (so that srwr is found by name, as an installed one is).
//
// exec.Command looks the name up on the PATH of this process, not of the command, so a name that is in binDir is given as its path.
func runIn(dir, binDir string, name string, args ...string) (string, error) {
	if p := filepath.Join(binDir, name); fileExists(p) {
		name = p
	}
	cmd := exec.Command(name, args...) //nolint:gosec // srwr of this repository, or go
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	if err != nil {
		out.WriteString("\n（失敗：" + err.Error() + "）\n")
	}
	return out.String(), err
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func fileText(workspace, rel string) string {
	b, _ := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(rel))) //nolint:gosec // a file of the workspace of this tool
	return string(b)
}

// initTryReport is the marks for the page.
func initTryReport(workspace, init1, init2 string, init1Err error, before2 map[string]string, tapesOut string, goTestErr error) (lines []string, ok bool) {
	ok = true
	check := func(cond bool, format string, a ...any) {
		mark := "○"
		if !cond {
			mark, ok = "×", false
		}
		lines = append(lines, mark+" "+fmt.Sprintf(format, a...))
	}
	mcp, settings := fileText(workspace, ".mcp.json"), fileText(workspace, ".claude/settings.json")
	check(init1Err == nil && strings.Contains(mcp, `"srwr"`) && strings.Contains(settings, "srwr hook"), "srwr init（1回目）が成功し、.mcp.json に srwr mcp、.claude/settings.json に hook が入った")
	check(strings.Contains(mcp, `"other"`) && strings.Contains(settings, `"SRWR_TRY"`) && strings.Contains(settings, "Bash(rm:*)"),
		"もとからあった設定（サーバー other、env、Bash(rm:*) の禁止）が残っている")
	check(strings.Contains(settings, `"Edit"`) && strings.Contains(settings, `"Write"`), "厳格モード：Edit・Write などの禁止が入った")
	same := true
	for name, text := range before2 {
		if fileText(workspace, name) != text {
			same = false
		}
	}
	check(same && strings.Contains(init2, "すでに準備できています"), "srwr init（2回目）は何も書き換えない")
	_, hasBackup := os.Stat(filepath.Join(workspace, ".srwr", "init-backup")) //nolint:gosec // a path in the workspace of this tool
	check(hasBackup == nil, "書き換える前の内容が .srwr/init-backup/ に残っている")

	files, _ := filepath.Glob(filepath.Join(workspace, ".srwr", "tapes", "*"+tape.FileSuffix))
	sort.Strings(files)
	if len(files) != 1 {
		return append(lines, fmt.Sprintf("× テープが %d 本できている（1本のはず）", len(files))), false
	}
	b, err := os.ReadFile(files[0]) //nolint:gosec // found by Glob in the workspace of this tool
	if err != nil {
		return append(lines, "× テープを読めない："+err.Error()), false
	}
	id := strings.TrimSuffix(filepath.Base(files[0]), tape.FileSuffix)
	n := map[string]int{}
	whyless := 0
	state := tape.NewState()
	for _, e := range tape.Parse(b).Events {
		state.Apply(e)
		if e.Type != tape.TypeLook && e.Type != tape.TypeEdit {
			continue
		}
		src := e.Source
		if src == "" {
			src = tape.SourceMCP
		}
		n[e.Type+"/"+src]++
		if src == tape.SourceMCP && (e.Why == nil || strings.TrimSpace(*e.Why) == "") {
			whyless++
		}
	}
	check(n["look/hook"] > 0, "AI がファイルを読んだ・探した記録（hook）がある：%d 件", n["look/hook"])
	check(n["edit/hook"] == 0, "Edit は禁止されていて、Edit の記録はない：%d 件", n["edit/hook"])
	check(n["look/mcp"] > 0 && n["edit/mcp"] > 0 && whyless == 0, "AI が srwr の look / edit で直した記録が、すべて理由つきである：look %d 件、edit %d 件", n["look/mcp"], n["edit/mcp"])
	same = true
	for name, f := range state.Files {
		if !f.Deleted && fileText(workspace, name) != f.Text {
			same = false
		}
	}
	check(same, "テープを最初から再生すると、最後のファイルの内容が実際のファイルと同じになる")
	check(strings.Contains(tapesOut, id), "srwr tapes の一覧に、このテープ（%s）が出る", id)
	check(goTestErr == nil, "直したあと go test ./... が通る")
	return lines, ok
}

// runInitTry is `ui-check init-try`.
func runInitTry(root string, out io.Writer) error {
	bin := filepath.Join(root, "bin", "srwr")
	binDir := filepath.Dir(bin)
	base := os.Getenv("SRWR_UI_OPEN_DIR")
	if base == "" {
		base = filepath.Join(os.TempDir(), "srwr-ui-open")
	}
	workspace := filepath.Join(base, "init-try")
	if err := prepareInitTry(workspace); err != nil {
		return err
	}
	claude, found := claudeBinary()
	if !found {
		return errors.New("claude（Claude Code の実行ファイル）が見つかりません。この文を AI に伝えてください")
	}
	init1, init1Err := runIn(workspace, binDir, "srwr", "init")
	before2 := map[string]string{}
	for _, name := range []string{".mcp.json", ".claude/settings.json", ".gitignore"} {
		before2[name] = fileText(workspace, name)
	}
	init2, _ := runIn(workspace, binDir, "srwr", "init")

	_, _ = fmt.Fprintln(out, "Claude Code に作業させています（数分かかります）…")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, claude, initTryClaudeArgs()...) //nolint:gosec // Claude Code, found above
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	var said bytes.Buffer
	cmd.Stdout, cmd.Stderr = &said, &said
	runErr := cmd.Run()

	tapesOut, _ := runIn(workspace, binDir, "srwr", "tapes")
	_, goTestErr := runIn(workspace, binDir, "go", "test", "./...")
	lines, allOK := initTryReport(workspace, init1, init2, init1Err, before2, tapesOut, goTestErr)
	shownOutputs := []shown{
		{"srwr init（1回目）の出力", init1},
		{"srwr init（2回目）の出力", init2},
		{"srwr tapes の出力", tapesOut},
		{".mcp.json（init のあと）", fileText(workspace, ".mcp.json")},
		{".claude/settings.json（init のあと）", fileText(workspace, ".claude/settings.json")},
	}
	page := filepath.Join(root, "ui-check-result", "init-try", "index.html")
	if err := os.MkdirAll(filepath.Dir(page), 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(page, []byte(renderTryPage("srwr init の確認", lines, shownOutputs, hookTryRows(workspace), said.String(), runErr)), 0o600); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "確認ページ：%s\n", page)
	if !allOK || runErr != nil {
		return errors.New("足りないものがあります。ページを見てください")
	}
	_, _ = fmt.Fprintln(out, "全部 ○ でした")
	return nil
}
