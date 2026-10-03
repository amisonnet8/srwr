package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/amisonnet8/srwr/internal/tape"
)

// hook-try is the human check of R7: Claude Code works in a workspace made for it, srwr mcp and srwr hook record, and this
// command checks the tape by itself (what is in it, that the lines of the edits are right, that it replays to the files). The
// person starts it, lets Claude Code work, and reads the marks. Everything but Claude Code's own work is done by this command.

const hookTryTask = `# 作業

このディレクトリの Go のテストが2つ落ちています。原因を調べて直してください。

1. まず、落ちているテストと、関係するファイルを、Read・Grep・Bash（cat、sed -n、grep -n）で調べる
2. text.go の Truncate は、srwr の select と replace（MCP ツール）で直す。why は日本語で書く
3. pad.go の PadLeft は、Edit で直す
4. 最後に go test ./... を実行して、通ることを確かめる
`

// hookTryAllow are the tools Claude Code may use without asking. They are in the settings of the workspace, and also on the
// command line: Claude Code ignores the permissions of the settings of a workspace that is not trusted yet (this one is new
// every time), but not the ones on the command line.
var hookTryAllow = []string{
	"mcp__srwr__select", "mcp__srwr__replace", "Read", "Edit", "Grep",
	"Bash(cat:*)", "Bash(nl:*)", "Bash(head:*)", "Bash(tail:*)", "Bash(sed:*)", "Bash(grep:*)", "Bash(go test:*)", "Bash(ls:*)",
}

// claudeArgs is the command line of Claude Code for the task: it works without asking, with the MCP server of .mcp.json.
func claudeArgs() []string {
	return []string{"-p", "Do what TASK.md says", "--allowedTools", strings.Join(hookTryAllow, ","), "--mcp-config", ".mcp.json"}
}

var hookTryFiles = map[string]string{
	"go.mod": "module trial\n\ngo 1.21\n",
	"text.go": `package trial

// Truncate cuts s to at most n characters. A string that fits is returned as it is;
// a longer one is cut and ends with an ellipsis, so the result has n characters.
func Truncate(s string, n int) string {
	r := []rune(s)
	if len(r) < n {
		return s
	}
	return string(r[:n-1]) + "…"
}
`,
	"pad.go": `package trial

// PadLeft puts spaces on the left of s until it has n characters.
func PadLeft(s string, n int) string {
	for len([]rune(s)) < n-1 {
		s = " " + s
	}
	return s
}
`,
	"text_test.go": `package trial

import "testing"

func TestTruncate(t *testing.T) {
	for _, tc := range []struct {
		in   string
		n    int
		want string
	}{
		{"abc", 5, "abc"},
		{"abc", 3, "abc"},
		{"abcdef", 3, "ab…"},
	} {
		if got := Truncate(tc.in, tc.n); got != tc.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
	}
}

func TestPadLeft(t *testing.T) {
	for _, tc := range []struct {
		in   string
		n    int
		want string
	}{
		{"ab", 5, "   ab"},
		{"ab", 2, "ab"},
		{"abc", 2, "abc"},
	} {
		if got := PadLeft(tc.in, tc.n); got != tc.want {
			t.Errorf("PadLeft(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
	}
}
`,
	"TASK.md": hookTryTask,
}

// prepareHookTry makes a new workspace in dst for Claude Code: a small project with two bugs, srwr mcp registered in
// .mcp.json, and the hook and the permissions in .claude/settings.json. Edit and Write are not forbidden (the lenient
// mode): the task asks for both ways of editing, so that both are recorded.
func prepareHookTry(dst, bin string) error {
	if err := os.RemoveAll(dst); err != nil { //nolint:gosec // the workspace of this tool, under the temporary directory
		return err
	}
	for name, text := range hookTryFiles {
		p := filepath.Join(dst, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil { //nolint:gosec // inside the workspace of this tool
			return err
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil { //nolint:gosec // inside the workspace of this tool
			return err
		}
	}
	mcp := map[string]any{"mcpServers": map[string]any{"srwr": map[string]any{"command": bin, "args": []string{"mcp", "--root", dst}}}}
	settings := map[string]any{
		"enableAllProjectMcpServers": true,
		"permissions":                map[string]any{"allow": hookTryAllow},
		"hooks": map[string]any{"PostToolUse": []any{map[string]any{
			"matcher": "Read|Bash|Grep|Edit",
			"hooks":   []any{map[string]any{"type": "command", "command": bin + " hook"}},
		}}},
	}
	for name, v := range map[string]any{".mcp.json": mcp, filepath.Join(".claude", "settings.json"): settings} {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		p := filepath.Join(dst, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil { //nolint:gosec // inside the workspace of this tool
			return err
		}
		if err := os.WriteFile(p, append(b, '\n'), 0o600); err != nil { //nolint:gosec // inside the workspace of this tool
			return err
		}
	}
	return nil
}

// hookTryReport reads the tape of the workspace and says, in plain lines, what was recorded. ok is true when every kind of
// record the check is about is there.
func hookTryReport(workspace string) (lines []string, tapeID string, ok bool) {
	files, _ := filepath.Glob(filepath.Join(workspace, ".srwr", "tapes", "*"+tape.FileSuffix))
	sort.Strings(files)
	if len(files) != 1 {
		return []string{fmt.Sprintf("× テープが %d 本できている（1本のはず）", len(files))}, "", false
	}
	b, err := os.ReadFile(files[0]) //nolint:gosec // found by Glob in the workspace of this tool
	if err != nil {
		return []string{"× テープを読めない：" + err.Error()}, "", false
	}
	n := map[string]int{}
	tools := map[string]int{}
	events := tape.Parse(b).Events
	state := tape.NewState()
	badRanges := 0
	for _, e := range events {
		state.Apply(e)
		if e.Type == tape.TypeReplace && e.Source == tape.SourceHook {
			// The lines the replace says it wrote must be, in the file as it was then, the lines it wrote.
			if tape.RangeText(state.Files[e.File].Text, e.NewStartLine, e.NewEndLine) != e.NewText {
				badRanges++
			}
		}
		if e.Type != tape.TypeSelect && e.Type != tape.TypeReplace {
			continue
		}
		src := e.Source
		if src == "" {
			src = tape.SourceMCP
		}
		n[e.Type+"/"+src]++
		if src == tape.SourceHook {
			tools[e.HookTool]++
		}
	}
	ok = true
	check := func(cond bool, format string, a ...any) {
		mark := "○"
		if !cond {
			mark, ok = "×", false
		}
		lines = append(lines, mark+" "+fmt.Sprintf(format, a...))
	}
	id := strings.TrimSuffix(filepath.Base(files[0]), tape.FileSuffix)
	check(true, "記録（テープ）が1本できた：%s", id)
	check(n["select/hook"] > 0, "AI がファイルを読んだ・探した記録：%d 件（Read %d、Bash %d、Grep %d）", n["select/hook"], tools["Read"], tools["Bash"], tools["Grep"])
	check(n["replace/hook"] > 0, "AI が Edit（いつもの編集）で直した記録：%d 件", n["replace/hook"])
	check(n["select/mcp"] > 0 && n["replace/mcp"] > 0, "AI が srwr の select / replace で直した記録：select %d 件、replace %d 件", n["select/mcp"], n["replace/mcp"])
	same := true
	var differs []string
	for name, f := range state.Files {
		if f.Deleted {
			continue
		}
		if cur, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(name))); err != nil || string(cur) != f.Text { //nolint:gosec // a file the tape names, in the workspace of this tool
			same = false
			differs = append(differs, name)
		}
	}
	sort.Strings(differs)
	check(badRanges == 0, "Edit の記録の行番号が、直した行を指している（合わないもの %d 件）", badRanges)
	check(same, "テープを最初から再生すると、最後のファイルの内容が実際のファイルと同じになる%s", func() string {
		if same {
			return ""
		}
		return "（違うファイル：" + strings.Join(differs, ", ") + "）"
	}())
	return lines, id, ok
}

// claudeBinary finds Claude Code: `claude` on the PATH, the one that started this tool's shell, or the one of the VSCode extension.
func claudeBinary() (string, bool) {
	if p, err := exec.LookPath("claude"); err == nil {
		return p, true
	}
	if p := os.Getenv("CLAUDE_CODE_EXECPATH"); p != "" {
		if _, err := os.Stat(p); err == nil { //nolint:gosec // the path of the running Claude Code, from its environment
			return p, true
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	found, _ := filepath.Glob(filepath.Join(home, ".vscode-server", "extensions", "anthropic.claude-code-*", "resources", "native-binary", "claude"))
	sort.Strings(found)
	if len(found) == 0 {
		return "", false
	}
	return found[len(found)-1], true
}

// runHookTry is `ui-check hook-try`: it makes the workspace, lets Claude Code work in it (the person's own Claude Code, with the
// person's own sign-in: this runs in the person's terminal), checks the tape, and writes the page to look at.
func runHookTry(root string, out io.Writer) error {
	bin := filepath.Join(root, "bin", "srwr")
	base := os.Getenv("SRWR_UI_OPEN_DIR")
	if base == "" {
		base = filepath.Join(os.TempDir(), "srwr-ui-open")
	}
	workspace := filepath.Join(base, "hook-try")
	if err := prepareHookTry(workspace, bin); err != nil {
		return err
	}
	claude, ok := claudeBinary()
	if !ok {
		return errors.New("claude（Claude Code の実行ファイル）が見つかりません。この文を AI に伝えてください")
	}
	_, _ = fmt.Fprintln(out, "Claude Code に作業させています（数分かかります）…")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, claude, claudeArgs()...) //nolint:gosec // Claude Code, found above
	cmd.Dir = workspace
	var said bytes.Buffer
	cmd.Stdout, cmd.Stderr = &said, &said
	runErr := cmd.Run()

	lines, _, allOK := hookTryReport(workspace)
	page := filepath.Join(root, "ui-check-result", "hook-try", "index.html")
	if err := os.MkdirAll(filepath.Dir(page), 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(page, []byte(renderHookTryPage(lines, hookTryRows(workspace), said.String(), runErr)), 0o600); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "確認ページ：%s\n", page)
	if !allOK || runErr != nil {
		return errors.New("記録に足りないものがあります。ページを見てください")
	}
	_, _ = fmt.Fprintln(out, "全部 ○ でした")
	return nil
}

// hookTryRow is an event of the tape, as the page shows it.
type hookTryRow struct {
	Seq             int
	Kind, By, Where string
	Why             string
}

func hookTryRows(workspace string) []hookTryRow {
	files, _ := filepath.Glob(filepath.Join(workspace, ".srwr", "tapes", "*"+tape.FileSuffix))
	if len(files) != 1 {
		return nil
	}
	b, err := os.ReadFile(files[0]) //nolint:gosec // found by Glob in the workspace of this tool
	if err != nil {
		return nil
	}
	var rows []hookTryRow
	for _, e := range tape.Parse(b).Events {
		if e.Type == tape.TypeHeader {
			continue
		}
		r := hookTryRow{Seq: e.Seq, Kind: e.Type, Where: e.File}
		if e.Type == tape.TypeSelect || e.Type == tape.TypeReplace {
			r.Where = fmt.Sprintf("%s:%d-%d", e.File, e.StartLine, e.EndLine)
			r.By = "srwr（select / replace）"
			if e.Source == tape.SourceHook {
				r.By = "AI の道具 " + e.HookTool
			}
			if e.Why != nil {
				r.Why = *e.Why
			}
		}
		rows = append(rows, r)
	}
	return rows
}

func renderHookTryPage(lines []string, rows []hookTryRow, said string, runErr error) string {
	return renderTryPage("hook の確認", lines, nil, rows, said, runErr)
}

// shown is a piece of text the page shows as it is, under a heading (what a command printed, for example).
type shown struct{ Heading, Text string }

func renderTryPage(title string, lines []string, outputs []shown, rows []hookTryRow, said string, runErr error) string {
	var b strings.Builder
	allOK := runErr == nil
	for _, l := range lines {
		if strings.HasPrefix(l, "×") {
			allOK = false
		}
	}
	b.WriteString(`<!doctype html><html lang="ja"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>` + html.EscapeString(title) + `</title><style>` + pageCSS + `</style></head><body>` + "\n")
	if allOK {
		b.WriteString(`<div class="big ok">全部 ○ でした</div><p>確認は終わりです。結果をチャットで「OK」と伝えてください。</p>` + "\n")
	} else {
		b.WriteString(`<div class="big ng">足りないものがあります</div><p>この画面のまま、チャットで「NG」と伝えてください（AI が原因を調べます）。</p>` + "\n")
	}
	if runErr != nil {
		fmt.Fprintf(&b, `<p class="ng">Claude Code の実行が失敗しました：%s</p>`+"\n", html.EscapeString(runErr.Error()))
	}
	b.WriteString("<h2>AI の作業の記録の検査</h2><table><tr><th>結果</th><th>確かめたこと</th></tr>\n")
	for _, l := range lines {
		cls := "ok"
		if strings.HasPrefix(l, "×") {
			cls = "ng"
		}
		mark, text, _ := strings.Cut(l, " ")
		fmt.Fprintf(&b, `<tr class="%s"><td>%s</td><td>%s</td></tr>`+"\n", cls, html.EscapeString(mark), html.EscapeString(text))
	}
	b.WriteString("</table>\n")
	for _, o := range outputs {
		fmt.Fprintf(&b, "<h2>%s</h2><pre>%s</pre>\n", html.EscapeString(o.Heading), html.EscapeString(o.Text))
	}
	b.WriteString("<h2>記録された操作（上から順）</h2><table><tr><th>番号</th><th>種類</th><th>だれが</th><th>ファイル:行</th><th>理由（why）</th></tr>\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "<tr><td>%d</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>\n", r.Seq, html.EscapeString(r.Kind), html.EscapeString(r.By), html.EscapeString(r.Where), html.EscapeString(r.Why))
	}
	b.WriteString("</table>\n<h2>AI の最後の返事</h2><pre>" + html.EscapeString(said) + "</pre>\n</body></html>\n")
	return b.String()
}
