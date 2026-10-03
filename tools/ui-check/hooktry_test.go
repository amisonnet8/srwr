package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/core"
	"github.com/amisonnet8/srwr/internal/hook"
	"github.com/amisonnet8/srwr/internal/session"
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
	if lines, _, ok := hookTryReport(root); ok || !strings.Contains(strings.Join(lines, "\n"), "× AI の Edit") {
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
	for _, want := range []string{"調べた読み取り（hook の select）：1 件（Read 1", "AI の Edit（hook の replace）：1 件", "select 1 件、replace 1 件"} {
		if !strings.Contains(strings.Join(lines, "\n"), want) {
			t.Errorf("the report lacks %q:\n%s", want, strings.Join(lines, "\n"))
		}
	}
}
