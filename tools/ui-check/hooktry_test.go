package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/core"
	"github.com/amisonnet8/srwr/internal/hook"
	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/tape"
)

func TestPrepareHookTryRegistersMCPAndTheHook(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "w")
	if err := prepareHookTry(dst, "/opt/srwr"); err != nil {
		t.Fatal(err)
	}
	var mcp struct {
		McpServers map[string]struct {
			Command string
			Args    []string
		}
	}
	var settings struct {
		Hooks struct {
			PostToolUse []struct {
				Matcher string
				Hooks   []struct{ Type, Command string }
			}
		}
		Permissions struct{ Allow []string }
	}
	for name, v := range map[string]any{".mcp.json": &mcp, ".claude/settings.json": &settings} {
		b, err := os.ReadFile(filepath.Join(dst, name)) //nolint:gosec // a path in a temporary directory
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, v); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	srv := mcp.McpServers["srwr"]
	if srv.Command != "/opt/srwr" || strings.Join(srv.Args, " ") != "mcp --root "+dst {
		t.Errorf("mcp server = %+v", srv)
	}
	h := settings.Hooks.PostToolUse
	if len(h) != 1 || h[0].Matcher != "Read|Bash|Grep|Edit" || len(h[0].Hooks) != 1 || h[0].Hooks[0].Type != "command" || h[0].Hooks[0].Command != "/opt/srwr hook" {
		t.Errorf("hooks = %+v", h)
	}
	if !strings.Contains(strings.Join(settings.Permissions.Allow, " "), "mcp__srwr__select") {
		t.Errorf("srwr tools are not allowed: %v", settings.Permissions.Allow)
	}
	for _, f := range []string{"TASK.md", "text.go", "pad.go", "text_test.go", "go.mod"} {
		if _, err := os.Stat(filepath.Join(dst, f)); err != nil {
			t.Error(err)
		}
	}
	// It is made again from nothing every time.
	if err := os.WriteFile(filepath.Join(dst, "left-over.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepareHookTry(dst, "/opt/srwr"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "left-over.txt")); err == nil {
		t.Error("a file of the last run is still there")
	}
}

// The task has the two bugs it says: the tests fail as they are, and pass when the two lines are fixed.
func TestHookTryProjectHasTwoBugs(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go")
	}
	dst := filepath.Join(t.TempDir(), "w")
	if err := prepareHookTry(dst, "/opt/srwr"); err != nil {
		t.Fatal(err)
	}
	test := func() (string, error) {
		cmd := exec.Command("go", "test", "./...")
		cmd.Dir = dst
		cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOWORK=off")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	out, err := test()
	if err == nil || !strings.Contains(out, "TestTruncate") || !strings.Contains(out, "TestPadLeft") {
		t.Fatalf("the tests should fail in both places:\n%s", out)
	}
	fix := func(file, old, repl string) {
		p := filepath.Join(dst, file)
		b, err := os.ReadFile(p) //nolint:gosec // a path in a temporary directory
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), old) {
			t.Fatalf("%s lacks %q", file, old)
		}
		if err := os.WriteFile(p, []byte(strings.Replace(string(b), old, repl, 1)), 0o600); err != nil { //nolint:gosec // a path in a temporary directory
			t.Fatal(err)
		}
	}
	fix("text.go", "len(r) < n {", "len(r) <= n {")
	fix("pad.go", "< n-1 {", "< n {")
	if out, err := test(); err != nil {
		t.Errorf("the tests should pass when the two lines are fixed:\n%s", out)
	}
}

func TestHookTryReport(t *testing.T) {
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	if err := os.WriteFile(filepath.Join(root, "f.go"), []byte("a\nb\nc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if lines, _, ok := hookTryReport(root); ok || len(lines) != 1 {
		t.Errorf("no tape: %v %v", lines, ok)
	}
	ws, err := session.Open(root, session.Options{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	c := &core.Core{WS: ws}
	call := func(tool string, input, resp map[string]any) {
		b, _ := json.Marshal(map[string]any{"hook_event_name": "PostToolUse", "cwd": root, "tool_name": tool, "tool_input": input, "tool_response": resp})
		if _, err := hook.Run(strings.NewReader(string(b)), c); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(root, "f.go")
	call("Read", map[string]any{"file_path": file}, nil)
	if lines, _, ok := hookTryReport(root); ok || !strings.Contains(strings.Join(lines, "\n"), "× AI が Edit（いつもの編集）で直した記録") {
		t.Errorf("only a read: %v %v", lines, ok)
	}
	sel, cerr := c.Select(core.SelectInput{File: "f.go", StartLine: 2, EndLine: 2, Why: "見る"})
	if cerr != nil {
		t.Fatal(cerr)
	}
	if _, cerr := c.Replace(core.ReplaceInput{Selection: sel.Selection, NewText: "B", Why: "直す"}); cerr != nil {
		t.Fatal(cerr)
	}
	if err := os.WriteFile(file, []byte("a\nB\nC\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	call("Edit", map[string]any{"file_path": file, "old_string": "c", "new_string": "C"}, map[string]any{"originalFile": "a\nB\nc\n"})
	lines, id, ok := hookTryReport(root)
	if !ok || id == "" {
		t.Errorf("everything is recorded, yet: %v %v", lines, ok)
	}
	for _, want := range []string{"Edit の記録の行番号が、直した行を指している（合わないもの 0 件）", "最後のファイルの内容が実際のファイルと同じになる", "AI がファイルを読んだ・探した記録：1 件（Read 1", "AI が Edit（いつもの編集）で直した記録：1 件", "select 1 件、replace 1 件"} {
		if !strings.Contains(strings.Join(lines, "\n"), want) {
			t.Errorf("the report lacks %q:\n%s", want, strings.Join(lines, "\n"))
		}
	}
}

// A tape that does not replay to the files, or an Edit whose lines are wrong, shows as × in the report.
func TestHookTryReportFindsAWrongTape(t *testing.T) {
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	file := filepath.Join(root, "f.go")
	if err := os.WriteFile(file, []byte("a\nb\nc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ws, err := session.Open(root, session.Options{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	c := &core.Core{WS: ws}
	b, _ := json.Marshal(map[string]any{"hook_event_name": "PostToolUse", "cwd": root, "tool_name": "Read", "tool_input": map[string]any{"file_path": file}})
	if _, err := hook.Run(strings.NewReader(string(b)), c); err != nil {
		t.Fatal(err)
	}
	// someone changes the file behind the tape's back, and nothing looks at it again
	if err := os.WriteFile(file, []byte("a\nB\nc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lines, _, ok := hookTryReport(root)
	if ok || !strings.Contains(strings.Join(lines, "\n"), "× テープを最初から再生すると") || !strings.Contains(strings.Join(lines, "\n"), "f.go") {
		t.Errorf("a tape that does not replay to the file: %v %v", lines, ok)
	}
}

func TestHookTryReportFindsAnEditWithWrongLines(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "f.go"), []byte("a\nB\nc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	text := "a\nb\nc\n"
	var tapeText strings.Builder
	for _, e := range []tape.Event{
		{Type: tape.TypeHeader, Session: "x", StartedAt: "2026-10-03T00:00:00.000+09:00", Author: &tape.Author{Kind: "ai", Name: "claude"}},
		{Type: tape.TypeSnapshot, Seq: 1, TS: "2026-10-03T00:00:00.000+09:00", File: "f.go", FileHash: tape.FileHash("f.go"), Text: &text, Sha: tape.Sha(text)},
		// the line written is line 2, but the event says 3
		{Type: tape.TypeReplace, Seq: 2, TS: "2026-10-03T00:00:00.000+09:00", File: "f.go", StartLine: 2, EndLine: 2, OldText: "b", NewText: "B", NewStartLine: 3, NewEndLine: 3, Source: tape.SourceHook, HookTool: "Edit"},
	} {
		line, err := tape.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		tapeText.Write(line)
	}
	dir := filepath.Join(root, ".srwr", "tapes")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "20261003-0000-x"+tape.FileSuffix), []byte(tapeText.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	lines, _, ok := hookTryReport(root)
	if ok || !strings.Contains(strings.Join(lines, "\n"), "× Edit の記録の行番号が、直した行を指している（合わないもの 1 件）") {
		t.Errorf("%v %v", lines, ok)
	}
}

func TestRenderHookTryPage(t *testing.T) {
	ok := []string{"○ 記録（テープ）が1本できた：x", "○ AI が Edit（いつもの編集）で直した記録：1 件"}
	rows := []hookTryRow{{1, "snapshot", "", "f.go", ""}, {2, "select", "AI の道具 Read", "f.go:1-3", ""}, {3, "replace", "srwr（select / replace）", "f.go:2-2", "直す <理由>"}}
	good := renderHookTryPage(ok, rows, "done <b>", nil)
	for _, want := range []string{"全部 ○ でした", "「OK」", "AI の道具 Read", "f.go:1-3", "直す &lt;理由&gt;", "done &lt;b&gt;"} {
		if !strings.Contains(good, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	bad := renderHookTryPage(append(ok, "× AI がファイルを読んだ・探した記録：0 件"), rows, "", nil)
	if !strings.Contains(bad, "足りないものがあります") || !strings.Contains(bad, "「NG」") || strings.Contains(bad, "全部 ○ でした") {
		t.Error("a × must make the page say NG")
	}
	failed := renderHookTryPage(ok, rows, "", errors.New("exit status 1"))
	if !strings.Contains(failed, "足りないものがあります") || !strings.Contains(failed, "exit status 1") {
		t.Error("a Claude Code that failed must make the page say NG and why")
	}
}

func TestClaudeBinaryIsFoundInTheVSCodeExtension(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("CLAUDE_CODE_EXECPATH", "")
	if _, ok := claudeBinary(); ok {
		t.Fatal("found a claude that is not there")
	}
	for _, v := range []string{"2.1.9", "2.1.287"} {
		dir := filepath.Join(home, ".vscode-server", "extensions", "anthropic.claude-code-"+v+"-linux-x64", "resources", "native-binary")
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "claude"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, ok := claudeBinary()
	if !ok || !strings.Contains(got, "claude-code-2.1.9") && !strings.Contains(got, "claude-code-2.1.287") {
		t.Errorf("claude = %q %v", got, ok)
	}
	t.Setenv("CLAUDE_CODE_EXECPATH", filepath.Join(home, "elsewhere"))
	if err := os.WriteFile(filepath.Join(home, "elsewhere"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _ := claudeBinary(); got != filepath.Join(home, "elsewhere") {
		t.Errorf("the path of the running Claude Code comes first: %q", got)
	}
}

// Claude Code ignores the permissions of an untrusted workspace's settings (R7: "Permission was declined"), so they are on
// the command line too, and the MCP server is named on it.
func TestClaudeArgsAllowTheToolsOnTheCommandLine(t *testing.T) {
	args := claudeArgs()
	joined := strings.Join(args, " ")
	for _, want := range []string{"-p Do what TASK.md says", "--mcp-config .mcp.json", "mcp__srwr__select", "mcp__srwr__replace", "Edit", "Bash(go test:*)"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the arguments lack %q: %v", want, args)
		}
	}
	i := slices.Index(args, "--allowedTools")
	if i < 0 || i+1 >= len(args) || args[i+1] != strings.Join(hookTryAllow, ",") {
		t.Errorf("--allowedTools does not carry the list of the settings: %v", args)
	}
}
