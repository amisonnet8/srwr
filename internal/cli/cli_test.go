package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func run(args []string, stdin string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = Run(args, strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string // a substring
		wantStderr string // a substring
	}{
		{"no arguments", nil, 2, "", "使い方"},
		{"--help", []string{"--help"}, 0, "srwr mcp", ""},
		{"-h", []string{"-h"}, 0, "srwr mcp", ""},
		{"--version", []string{"--version"}, 0, "srwr (devel)", ""},
		{"unknown command", []string{"frobnicate"}, 2, "", `知らないコマンド "frobnicate"`},
		{"init is not made yet", []string{"init"}, 2, "", "まだ実装されていません"},
		{"hook with an unknown flag", []string{"hook", "--nope"}, 1, "", "nope"},
		{"hook with a stray argument", []string{"hook", "x"}, 1, "", ""},
		{"hook with a root that is missing does not stop the agent", []string{"hook", "--root", "/no/such/dir/at/all"}, 0, "", "ディレクトリとして開けない"},
		{"view-server with a root that is missing", []string{"view-server", "--root", "/no/such/dir/at/all"}, 1, "", "ディレクトリとして開けません"},
		{"view-server with a stray argument", []string{"view-server", "x"}, 2, "", "余分な引数"},
		{"mcp with an unknown flag", []string{"mcp", "--nope"}, 2, "", "nope"},
		{"mcp with a stray argument", []string{"mcp", "x"}, 2, "", "余分な引数"},
		{"mcp with a root that is missing", []string{"mcp", "--root", "/no/such/dir/at/all"}, 1, "", "ディレクトリとして開けません"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := run(tt.args, "")
			if code != tt.wantCode || !strings.Contains(stdout, tt.wantStdout) || !strings.Contains(stderr, tt.wantStderr) {
				t.Errorf("code = %d, stdout = %q, stderr = %q; want %d, %q, %q", code, stdout, stderr, tt.wantCode, tt.wantStdout, tt.wantStderr)
			}
		})
	}
}

func TestHookNeverStopsTheAgent(t *testing.T) {
	root := t.TempDir()
	for name, in := range map[string]string{
		"empty":           "",
		"not json":        "hello",
		"a wrong shape":   `{"hook_event_name":"PostToolUse","tool_name":"Read","tool_input":"x"}`,
		"a missing file":  `{"hook_event_name":"PostToolUse","tool_name":"Read","tool_input":{"file_path":"` + filepath.ToSlash(filepath.Join(root, "nope.txt")) + `"}}`,
		"an outside path": `{"hook_event_name":"PostToolUse","tool_name":"Read","tool_input":{"file_path":"/etc/passwd"}}`,
		"another event":   `{"hook_event_name":"PreToolUse","tool_name":"Read"}`,
	} {
		code, stdout, _ := run([]string{"hook", "--root", root}, in)
		if code != 0 || stdout != "" {
			t.Errorf("%s: code %d, stdout %q (exit 2 would stop the agent's tool call)", name, code, stdout)
		}
	}
	if m, _ := filepath.Glob(filepath.Join(root, ".srwr", "tapes", "*")); len(m) != 0 {
		t.Errorf("a tape was made although nothing could be recorded: %v", m)
	}
}

func TestMCPEndsWhenInputEnds(t *testing.T) {
	code, stdout, _ := run([]string{"mcp", "--root", t.TempDir()}, `{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n")
	if code != 0 || strings.TrimSpace(stdout) != `{"jsonrpc":"2.0","id":1,"result":{}}` {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
}

func TestViewServerEndsWhenInputEnds(t *testing.T) {
	code, stdout, _ := run([]string{"view-server", "--root", t.TempDir()},
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"client":"vim","protocolVersion":1}}`+"\n"+
			`{"jsonrpc":"2.0","id":2,"method":"tapes/list","params":{}}`+"\n")
	if code != 0 || !strings.Contains(stdout, `"tapes":[]`) {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
}
