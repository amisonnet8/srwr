package cli

import (
	"bytes"
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
		{"hook is not made yet", []string{"hook"}, 2, "", "まだ実装されていません"},
		{"view-server is not made yet", []string{"view-server"}, 2, "", "まだ実装されていません"},
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

func TestMCPEndsWhenInputEnds(t *testing.T) {
	code, stdout, _ := run([]string{"mcp", "--root", t.TempDir()}, `{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n")
	if code != 0 || strings.TrimSpace(stdout) != `{"jsonrpc":"2.0","id":1,"result":{}}` {
		t.Errorf("code = %d, stdout = %q", code, stdout)
	}
}
