// Package cli is the subcommands of srwr (docs/reference/cli.md). cmd/srwr only hands over the arguments.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/amisonnet8/srwr/internal/core"
	"github.com/amisonnet8/srwr/internal/mcp"
	"github.com/amisonnet8/srwr/internal/session"
)

const usage = `srwr: AI に select / replace の2コマンドだけでファイルを編集させ、操作をテープに記録する

使い方:
  srwr mcp [--root <作業場>]     MCP サーバー（select / replace）。AI のエージェントが起動する
  srwr hook                      Claude Code の hook の記録
  srwr view-server               表示サーバー（エディタが起動する）
  srwr view [テープ]             Vim で再生する
  srwr init                      作業場を srwr 用に準備する
  srwr tapes                     テープの一覧・整理
  srwr --version                 バージョン
  srwr --help                    この説明

まだ使えるのは mcp だけです。
`

// notYet are the subcommands that exist in docs/reference/cli.md and are made in later stages.
var notYet = map[string]bool{"hook": true, "view-server": true, "view": true, "init": true, "tapes": true}

// Run runs srwr with the arguments (without the program name) and returns the exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	switch cmd := args[0]; {
	case cmd == "--help" || cmd == "-h" || cmd == "help":
		_, _ = io.WriteString(stdout, usage)
		return 0
	case cmd == "--version" || cmd == "version":
		_, _ = fmt.Fprintf(stdout, "srwr %s\n", Version())
		return 0
	case cmd == "mcp":
		return runMCP(args[1:], stdin, stdout, stderr)
	case notYet[cmd]:
		_, _ = fmt.Fprintf(stderr, "srwr %s: まだ実装されていません\n", cmd)
		return 2
	default:
		_, _ = fmt.Fprintf(stderr, "srwr: 知らないコマンド %q\n\n%s", cmd, usage)
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

func runMCP(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("srwr mcp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "作業場のディレクトリ")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "srwr mcp: 余分な引数 %q\n", fs.Arg(0))
		return 2
	}
	if info, err := os.Stat(*root); err != nil || !info.IsDir() {
		_, _ = fmt.Fprintf(stderr, "srwr mcp: 作業場 %q がディレクトリとして開けません\n", *root)
		return 1
	}

	ws, err := session.Open(*root, session.Options{Version: Version()})
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
