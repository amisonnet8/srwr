package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

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
		"permissions": map[string]any{"allow": []string{
			"mcp__srwr__select", "mcp__srwr__replace", "Read", "Edit", "Grep",
			"Bash(cat:*)", "Bash(nl:*)", "Bash(head:*)", "Bash(tail:*)", "Bash(sed:*)", "Bash(grep:*)", "Bash(go test:*)", "Bash(ls:*)",
		}},
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

// runHookTry is `ui-check hook-try`.
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
	if _, err := exec.LookPath("code"); err != nil {
		return fmt.Errorf("the code command was not found. Open %s in VSCode by hand", workspace)
	}
	if err := command(root, out, "code", "-n", workspace); err != nil {
		return fmt.Errorf("opening VSCode: %w", err)
	}
	_, _ = fmt.Fprintf(out, "\n作業場：%s\n\n【手順】新しい VSCode が開きます。そこで次のようにします（日本語入力は切る）\n", workspace)
	for i, s := range hookTrySteps {
		_, _ = fmt.Fprintf(out, "%d. %s\n", i+1, s)
	}
	_, _ = fmt.Fprint(out, "\nAI の作業が終わったら、このターミナルに戻って Enter を押してください: ")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')

	lines, _, ok := hookTryReport(workspace)
	_, _ = fmt.Fprintln(out, "\n【記録されたもの】")
	for _, l := range lines {
		_, _ = fmt.Fprintln(out, l)
	}
	if !ok {
		return fmt.Errorf("記録に足りないものがあります（上の × ）。この結果を AI に伝えてください")
	}
	_, _ = fmt.Fprintln(out, "\n全部 ○ なら、確認は終わりです。結果をチャットで教えてください。")
	return nil
}

var hookTrySteps = []string{
	"左端の Claude Code のアイコンを押して、新しい会話を開く（このフォルダを信頼するか聞かれたら「信頼する」）",
	"「MCP サーバー srwr を使うか」と聞かれたら、許可する",
	"入力欄に英数字で `Do what TASK.md says` と打って Enter を押す",
	"AI が作業を終えて、`go test` が通ったと言うまで待つ。許可を聞かれたら、許可する",
}
