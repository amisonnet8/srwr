// Package cli is the subcommands of srwr (docs/reference/cli.md). cmd/srwr only hands over the arguments.
package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/amisonnet8/srwr/internal/core"
	"github.com/amisonnet8/srwr/internal/hook"
	"github.com/amisonnet8/srwr/internal/lang"
	"github.com/amisonnet8/srwr/internal/mcp"
	"github.com/amisonnet8/srwr/internal/session"
	"github.com/amisonnet8/srwr/internal/tape"
	"github.com/amisonnet8/srwr/internal/viewserver"
)

const usageEN = `srwr: let an AI edit files with just two commands, select / replace, and record the operations on a tape

Usage:
  srwr mcp [--root <workspace>]          MCP server (look / edit / new). Started by the AI agent
  srwr hook [--root <workspace>]         Record Claude Code hook events (reads JSON on stdin)
  srwr view-server [--root <workspace>]  View server (started by the editor)
  srwr view [tape] [--live]              Replay in Vim (--root <workspace>)
  srwr init [--lenient]                  Set up a workspace for srwr (--lenient: do not forbid Edit and Write; --root <workspace>)
  srwr tapes [new|prune|path]            List and tidy tapes (--root <workspace>)
  srwr trace [--mark] [file]             Find the tape operations (and why) behind the lines a diff adds (git show | srwr trace)
  srwr --version                         Version
  srwr --help                            This help
`

const usageJA = `srwr: AI に select / replace の2コマンドだけでファイルを編集させ、操作をテープに記録する

使い方:
  srwr mcp [--root <作業場>]          MCP サーバー（look / edit / new）。AI のエージェントが起動する
  srwr hook [--root <作業場>]         Claude Code の hook の記録（標準入力の JSON を読む）
  srwr view-server [--root <作業場>]  表示サーバー（エディタが起動する）
  srwr view [テープ] [--live]         Vim で再生する（--root <作業場>）
  srwr init [--lenient]               作業場を srwr 用に準備する（--lenient：Edit・Write を禁止しない。--root <作業場>）
  srwr tapes [new|prune|path]         テープの一覧・整理（--root <作業場>）
  srwr trace [--mark] [file]             差分（git show など）の追加行を作ったテープの操作と理由を引く（git show | srwr trace）
  srwr --version                      バージョン
  srwr --help                         この説明
`

// usage is the help text in the language the person asked for.
func usage() string { return lang.Pick(usageEN, usageJA) }

// Run runs srwr with the arguments (without the program name) and returns the exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = io.WriteString(stderr, usage())
		return 2
	}
	switch cmd := args[0]; cmd {
	case "--help", "-h", "help":
		_, _ = io.WriteString(stdout, usage())
		return 0
	case "--version", "version":
		_, _ = fmt.Fprintf(stdout, "srwr %s\n", Version())
		return 0
	case "mcp":
		return runMCP(args[1:], stdin, stdout, stderr)
	case "hook":
		return runHook(args[1:], stdin, stdout, stderr)
	case "view-server":
		return runViewServer(args[1:], stdin, stdout, stderr)
	case "tapes":
		return runTapes(args[1:], stdout, stderr)
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "view":
		return runView(args[1:], stdin, stdout, stderr)
	case "trace":
		return runTrace(args[1:], stdin, stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, lang.Pick("srwr: unknown command %q\n\n%s", "srwr: 知らないコマンド %q\n\n%s"), cmd, usage())
		return 2
	}
}

// Version returns the version of the module srwr was installed as (go install …@v1.2.3), or
// "(devel)" when the binary was built from a checkout.
func Version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

// workspaceArg reads --root and checks that it is a directory. It returns the code to exit with if it is not.
func workspaceArg(name string, args []string, stderr io.Writer) (root string, code int) {
	fs := flag.NewFlagSet("srwr "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	r := fs.String("root", ".", lang.Pick("the workspace directory", "作業場のディレクトリ"))
	if err := fs.Parse(args); err != nil {
		return "", 2
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, lang.Pick("srwr %s: unexpected argument %q\n", "srwr %s: 余分な引数 %q\n"), name, fs.Arg(0))
		return "", 2
	}
	if info, err := os.Stat(*r); err != nil || !info.IsDir() {
		_, _ = fmt.Fprintf(stderr, lang.Pick("srwr %s: cannot open the workspace %q as a directory\n", "srwr %s: 作業場 %q がディレクトリとして開けません\n"), name, *r)
		return "", 1
	}
	return *r, 0
}

func runViewServer(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	root, code := workspaceArg("view-server", args, stderr)
	if code != 0 {
		return code
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "srwr view-server: %v\n", err)
		return 1
	}
	if err := (&viewserver.Server{Root: abs, Version: Version()}).Serve(stdin, stdout); err != nil {
		_, _ = fmt.Fprintf(stderr, "srwr view-server: %v\n", err)
		return 1
	}
	return 0
}

// Now is the clock of the commands that open the workspace. Tests replace it; nothing else does.
var Now = time.Now

func runMCP(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	root, code := workspaceArg("mcp", args, stderr)
	if code != 0 {
		return code
	}

	ws, err := session.Open(root, session.Options{Version: Version(), Now: Now})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "srwr mcp: %v\n", err)
		return 1
	}
	server := &mcp.Server{Core: &core.Core{WS: ws}, Version: Version()}
	if err := server.Serve(stdin, stdout); err != nil {
		_, _ = fmt.Fprintf(stderr, "srwr mcp: %v\n", err)
		return 1
	}
	return 0
}

// runHook records what Claude Code tells a hook. It never gets in the agent's way: whatever goes wrong it only says so on the
// standard error output and exits with 0 (an exit code of 2 would stop the agent's tool call). A wrong command line is the
// one thing that exits with 1, which the agent sees as a failed hook and goes on.
func runHook(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("srwr hook", flag.ContinueOnError)
	fs.SetOutput(stderr)
	def := os.Getenv("CLAUDE_PROJECT_DIR")
	if def == "" {
		def = "."
	}
	root := fs.String("root", def, "the workspace directory (default: CLAUDE_PROJECT_DIR, or the current directory)")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		return 1
	}
	if info, err := os.Stat(*root); err != nil || !info.IsDir() { //nolint:gosec // the workspace the user (or Claude Code) named
		_, _ = fmt.Fprintf(stderr, "srwr hook: cannot open the workspace %q as a directory, so nothing is recorded\n", *root)
		return 0
	}
	ws, err := session.Open(*root, session.Options{Version: Version(), Now: Now})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "srwr hook: %v\n", err)
		return 0
	}
	ws.SetAuthor(tape.Author{Kind: "ai", Name: "claude"})
	notes, advice, err := hook.RunWithAdvice(stdin, &core.Core{WS: ws})
	for _, n := range notes {
		_, _ = fmt.Fprintf(stderr, "srwr hook: %s\n", n)
	}
	if len(advice) > 0 {
		// What Claude Code shows the agent after the tool: the JSON of a PostToolUse hook on the standard output (exit code 0).
		out := map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": "PostToolUse", "additionalContext": strings.Join(advice, "\n")}}
		if b, mErr := json.Marshal(out); mErr == nil {
			_, _ = fmt.Fprintf(stdout, "%s\n", b)
		}
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "srwr hook: %v\n", err)
	}
	return 0
}
