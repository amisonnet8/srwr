package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/core"
	"github.com/amisonnet8/srwr/internal/hook"
	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/setup"
)

func TestInitTryWorkspaceHasItsOwnSettingsAndTwoBugs(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "w")
	if err := prepareInitTry(dst); err != nil {
		t.Fatal(err)
	}
	if got := fileText(dst, ".mcp.json"); got != initTryMCP {
		t.Errorf(".mcp.json = %q", got)
	}
	if got := fileText(dst, ".claude/settings.json"); got != initTrySettings {
		t.Errorf("settings.json = %q", got)
	}
	for _, f := range []string{"TASK.md", "text.go", "pad.go", "text_test.go", "go.mod"} {
		if _, err := os.Stat(filepath.Join(dst, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	if !strings.Contains(fileText(dst, "TASK.md"), "Edit が使えなければ") {
		t.Error("the task does not say what to do when Edit is forbidden")
	}
	if strings.Contains(strings.Join(initTryAllow(), " "), "Edit") {
		t.Errorf("Edit is allowed on the command line, which would undo the ban of init: %v", initTryAllow())
	}
	args := initTryClaudeArgs()
	if !slices.Contains(args, "--mcp-config") || slices.Contains(args, "--settings") {
		t.Errorf("args = %v", args)
	}
}

func TestInitTryReport(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "w")
	if real, err := filepath.EvalSymlinks(filepath.Dir(dst)); err == nil {
		dst = filepath.Join(real, "w")
	}
	if err := prepareInitTry(dst); err != nil {
		t.Fatal(err)
	}
	lookNone := func(string) (string, error) { return "", errors.New("no") }
	if _, err := setup.Init(setup.Options{Root: dst, LookPath: lookNone}); err != nil {
		t.Fatal(err)
	}
	before := map[string]string{}
	for _, n := range []string{".mcp.json", ".claude/settings.json", ".gitignore"} {
		before[n] = fileText(dst, n)
	}
	if _, err := setup.Init(setup.Options{Root: dst, LookPath: lookNone}); err != nil {
		t.Fatal(err)
	}
	for n, text := range before {
		if fileText(dst, n) != text {
			t.Fatalf("%s changed on the second run", n)
		}
	}

	// A tape like the one Claude Code makes in the strict mode: reads through the hook, edits through select and replace.
	ws, err := session.Open(dst, session.Options{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	c := &core.Core{WS: ws}
	b, _ := json.Marshal(map[string]any{"hook_event_name": "PostToolUse", "cwd": dst, "tool_name": "Read", "tool_input": map[string]any{"file_path": filepath.Join(dst, "pad.go")}, "tool_response": nil})
	if _, err := hook.Run(strings.NewReader(string(b)), c); err != nil {
		t.Fatal(err)
	}
	sel, cerr := c.Look(core.LookInput{File: "pad.go", StartLine: 1, EndLine: 1, Why: "見る"})
	if cerr != nil {
		t.Fatal(cerr)
	}
	if _, cerr := c.Edit(core.EditInput{Selection: sel.Selection, NewText: "package trial", Why: "直す"}); cerr != nil {
		t.Fatal(cerr)
	}
	id, _ := ws.Current()
	lines, ok := initTryReport(dst, "init out", "すでに準備できています。", nil, before, "  "+id+"  10/03", nil)
	if !ok {
		t.Errorf("everything is right, yet:\n%s", strings.Join(lines, "\n"))
	}
	if len(lines) != 11 {
		t.Errorf("%d marks, want 11:\n%s", len(lines), strings.Join(lines, "\n"))
	}

	// Each mark turns × when what it is about is wrong.
	for name, tc := range map[string]struct {
		init2     string
		init1Err  error
		tapesOut  string
		goTestErr error
		want      string
	}{
		"second run wrote": {"変更：.mcp.json", nil, id, nil, "× srwr init（2回目）は何も書き換えない"},
		"init failed":      {"すでに準備できています。", errors.New("x"), id, nil, "× srwr init（1回目）が成功し"},
		"tape not listed":  {"すでに準備できています。", nil, "テープはありません。", nil, "× srwr tapes の一覧に"},
		"go test failed":   {"すでに準備できています。", nil, id, errors.New("exit 1"), "× 直したあと go test"},
	} {
		lines, ok := initTryReport(dst, "", tc.init2, tc.init1Err, before, tc.tapesOut, tc.goTestErr)
		if ok || !strings.Contains(strings.Join(lines, "\n"), tc.want) {
			t.Errorf("%s: ok=%v\n%s", name, ok, strings.Join(lines, "\n"))
		}
	}

	// An Edit recorded by the hook (it should have been forbidden) is a ×.
	file := filepath.Join(dst, "pad.go")
	text := fileText(dst, "pad.go")
	edited := strings.Replace(text, "package trial", "package trial // x", 1)
	if err := os.WriteFile(file, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	b, _ = json.Marshal(map[string]any{"hook_event_name": "PostToolUse", "cwd": dst, "tool_name": "Edit",
		"tool_input": map[string]any{"file_path": file, "old_string": "package trial", "new_string": "package trial // x"}, "tool_response": map[string]any{"originalFile": text}})
	if _, err := hook.Run(strings.NewReader(string(b)), c); err != nil {
		t.Fatal(err)
	}
	lines, ok = initTryReport(dst, "", "すでに準備できています。", nil, before, id, nil)
	if ok || !strings.Contains(strings.Join(lines, "\n"), "× Edit は禁止されていて") {
		t.Errorf("an Edit was recorded, yet:\n%s", strings.Join(lines, "\n"))
	}
}

func TestInitTryPageShowsTheOutputs(t *testing.T) {
	page := renderTryPage("srwr init の確認", []string{"○ a"}, []shown{{"srwr init（1回目）の出力", "<b>x</b>"}}, nil, "said", nil)
	for _, want := range []string{"<title>srwr init の確認</title>", "srwr init（1回目）の出力", "&lt;b&gt;x&lt;/b&gt;", "全部 ○ でした"} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
}

// A name that is only in binDir (srwr is not installed on this machine) is found, and what a failed command said is kept.
func TestRunInFindsTheCommandInBinDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the command is a shell script")
	}
	bin, work := t.TempDir(), t.TempDir()
	script := "#!/bin/sh\necho \"hello from $PWD $1\"\nexit 3\n"
	if err := os.WriteFile(filepath.Join(bin, "srwrfake"), []byte(script), 0o700); err != nil { //nolint:gosec // a script in a temporary directory
		t.Fatal(err)
	}
	out, err := runIn(work, bin, "srwrfake", "init")
	if err == nil || !strings.Contains(out, "hello from") || !strings.Contains(out, "init") || !strings.Contains(out, "失敗：exit status 3") {
		t.Errorf("out %q err %v", out, err)
	}
	if _, err := runIn(work, bin, "no-such-command-at-all"); err == nil {
		t.Error("a command that does not exist succeeded")
	}
}
