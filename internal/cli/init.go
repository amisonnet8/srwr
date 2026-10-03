package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/amisonnet8/srwr/internal/setup"
)

// runInit is srwr init [--lenient] [--root <dir>]. It exits with 1 when it could not prepare the workspace; then it has
// written nothing if the mistake was in the person's files.
func runInit(args []string, stdout, stderr io.Writer) int {
	return initWith(args, stdout, stderr, setup.Options{})
}

func initWith(args []string, stdout, stderr io.Writer, opts setup.Options) int {
	fs := flag.NewFlagSet("srwr init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	lenient := fs.Bool("lenient", false, "Edit・Write を禁止しない（緩いモード）")
	root := fs.String("root", ".", "作業場のディレクトリ")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "srwr init: 余分な引数 %q\n", fs.Arg(0))
		return 2
	}
	if info, err := os.Stat(*root); err != nil || !info.IsDir() {
		_, _ = fmt.Fprintf(stderr, "srwr init: 作業場 %q がディレクトリとして開けません\n", *root)
		return 1
	}
	opts.Root, opts.Lenient = *root, *lenient
	res, err := setup.Init(opts)
	var user *setup.UserError
	switch {
	case errors.As(err, &user):
		_, _ = fmt.Fprintf(stderr, "srwr init: %s\n何も書き換えていません。直してから、もう一度実行してください。\n", user.Msg)
		return 1
	case err != nil:
		_, _ = fmt.Fprintf(stderr, "srwr init: %v\n", err)
		return 1
	}
	writeInitReport(stdout, res)
	return 0
}

var changeLabel = map[setup.Kind]string{setup.Created: "作った", setup.Appended: "追記した", setup.Unchanged: "変更なし"}

func writeInitReport(w io.Writer, r *setup.Result) {
	_, _ = fmt.Fprintf(w, "作業場：%s\n\n", r.Root)
	for _, c := range r.Changes {
		detail := c.Detail
		if c.Path == ".srwr/" && c.Kind == setup.Created {
			detail = "鍵 .srwr/key を作りました"
		}
		_, _ = fmt.Fprintln(w, strings.TrimRight("  "+padRight(changeLabel[c.Kind], 9)+padRight(c.Path, 22)+detail, " "))
	}
	_, _ = fmt.Fprintln(w)
	if !r.Changed() {
		_, _ = fmt.Fprintln(w, "すでに準備できています。何も書き換えませんでした。")
	} else {
		if r.Backup != "" {
			_, _ = fmt.Fprintf(w, "書き換える前の内容は %s/ に残しました。\n", r.Backup)
		}
		if r.RegistrationChanged {
			_, _ = fmt.Fprintln(w, "準備できました。Claude Code を開き直すと、select / replace が使えます。")
		}
		if r.Lenient {
			_, _ = fmt.Fprintln(w, "緩いモードでは、Edit は replace（理由なし）として記録され、Write は external として見えます。")
		} else if settingsCreated(r) {
			_, _ = fmt.Fprintln(w, "緩いモード（Edit・Write を禁止しない）にするときは、srwr init --lenient。")
		}
	}
	if !r.SrwrOnPath {
		_, _ = fmt.Fprintln(w, "注意：srwr が PATH にありません。.mcp.json と hook は srwr を呼ぶので、PATH に入れてください。")
	}
}

func settingsCreated(r *setup.Result) bool {
	for _, c := range r.Changes {
		if c.Path == ".claude/settings.json" {
			return c.Kind == setup.Created
		}
	}
	return false
}
