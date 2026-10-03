// Command ui-check prepares what a person needs to look at the UI by eye (docs: .claude/rules/working-with-human.md 3章).
// `ui-check run` (qsoku ui-check) does all the checks and makes the page to look at; `ui-check live [--watch <vscode|vim>]`
// (qsoku ui-live) is the same for the live view only; `ui-check accept [ng <note>]` (qsoku ui-accept) records the decision.
// `ui-check open <vscode|vim> <why-basic|external|no-why|live>` is called by `qsoku ui-open`.
// `ui-check vim-baseline <images> <out>` makes vim/test/baseline from the approved Vim images (done once, at R5).
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "ui-check:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) >= 1 {
		switch args[0] {
		case "run":
			return doRun(false, out)
		case "live":
			if len(args) == 3 && args[1] == "--watch" {
				return run([]string{"open", args[2], "live"}, out)
			}
			if len(args) != 1 {
				return errors.New("usage: ui-check live [--watch <vscode|vim>]")
			}
			return doRun(true, out)
		case "hook-try":
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			return runHookTry(root, out)
		case "accept":
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			if len(args) == 1 {
				return acceptResult(root, true, "", out)
			}
			if args[1] == "ng" && len(args) >= 3 {
				return acceptResult(root, false, strings.Join(args[2:], " "), out)
			}
			return errors.New("usage: ui-check accept [ng <note>]")
		}
	}
	if len(args) == 3 && args[0] == "vim-baseline" {
		return makeVimBaseline(args[1], args[2])
	}
	if len(args) != 3 || args[0] != "open" || (args[1] != "vscode" && args[1] != "vim") {
		return errors.New("usage: ui-check open <vscode|vim> <why-basic|external|no-why|long-why|live>")
	}
	name := args[2]
	check := name
	if args[1] == "vim" {
		check, _ = splitLight(name)
	}
	if _, ok := tapes[check]; !ok && check != "live" {
		return fmt.Errorf("unknown tape %q: why-basic, external, no-why, long-why or live (Vim: and the same with -light)", name)
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if args[1] == "vim" {
		return openVim(root, name, out)
	}
	return openVSCode(root, name, out)
}

func openVSCode(root, name string, out io.Writer) error {
	bin := filepath.Join(root, "bin", "srwr")
	fixture := filepath.Join(root, "extension", "test", "fixtures", "ui-check")
	base := os.Getenv("SRWR_UI_OPEN_DIR")
	if base == "" {
		base = filepath.Join(os.TempDir(), "srwr-ui-open")
	}
	workspace := filepath.Join(base, name)
	if err := prepareWorkspace(fixture, workspace, bin, name == "live"); err != nil {
		return err
	}
	if name == "long-why" {
		if err := addLongWhy(bin, workspace); err != nil {
			return err
		}
	}
	vsix := filepath.Join(base, "srwr-view.vsix")
	var vsceOut strings.Builder
	if err := command(filepath.Join(root, "extension"), &vsceOut, "npx", "--yes", "@vscode/vsce", "package", "--no-dependencies", "-o", vsix); err != nil {
		return fmt.Errorf("making the .vsix: %w\n%s", err, vsceOut.String())
	}
	if _, err := exec.LookPath("code"); err != nil {
		return fmt.Errorf("the code command was not found. Open %s in VSCode by hand and install %s", workspace, vsix)
	}
	if err := command(root, out, "code", "--install-extension", vsix, "--force"); err != nil {
		return fmt.Errorf("installing the extension: %w", err)
	}
	if err := command(root, out, "code", "-n", workspace); err != nil {
		return fmt.Errorf("opening VSCode: %w", err)
	}
	guide(out, name, workspace)
	if name != "live" {
		return nil
	}
	wait := func() {
		_, _ = fmt.Fprint(out, "「ライブ視聴を開始」を押したら Enter を押してください（そこから3秒ごとに1コマずつ追記します）: ")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
	return feedLive(workspace, fixture, wait, 3*time.Second, out)
}

func command(dir string, out io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...) //nolint:gosec // fixed commands: npx, code
	cmd.Dir = dir
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

func guide(out io.Writer, name, workspace string) {
	_, _ = fmt.Fprintf(out, "\n作業場：%s\n", workspace)
	if name == "live" {
		_, _ = fmt.Fprintln(out, "【ライブ】開いた VSCode で、左端のカセット →「ライブ視聴を開始」。")
		_, _ = fmt.Fprintln(out, "見るところ：追っている間は「● LIVE」で、ちらつかず、すぐ出る。「戻る」を押すと「LIVE に戻る（新着 N）」が橙で出る。")
		return
	}
	t := tapes[name]
	_, _ = fmt.Fprintf(out, "【%s】開いた VSCode で、左端のカセット →「テープを開く」→「%s」を選ぶ。\n", name, t.Started)
	_, _ = fmt.Fprintf(out, "見るところ：%s。パネル・下のバー・タブ・アイコンが崩れていないか。\n", t.Look)
	_, _ = fmt.Fprintln(out, "light で見るとき：Ctrl+K Ctrl+T →「Light Modern」。")
}

// doRun is `ui-check run` and `ui-check live`: it ends with the summary and the page to open. An exit code of 1 means a check
// failed or a screen differs from the baseline.
func doRun(liveOnly bool, out io.Writer) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	rep, dir, err := runCheck(root, liveOnly, out)
	if err != nil {
		return err
	}
	page := filepath.Join(root, "ui-check-result", "latest", "index.html")
	_, _ = fmt.Fprintln(out)
	failed := 0
	for _, c := range rep.Checks {
		if !c.OK {
			failed++
			_, _ = fmt.Fprintf(out, "失敗：%s：%s\n", c.Name, strings.Join(c.Notes, "；"))
		}
	}
	for _, p := range rep.Problems {
		_, _ = fmt.Fprintln(out, "問題："+p)
	}
	switch {
	case rep.Clean() && rep.NewCount() == 0:
		_, _ = fmt.Fprintln(out, "違いなし")
	case rep.Clean():
		_, _ = fmt.Fprintf(out, "違いなし。新しい画面が %d（基準がまだ無い）\n", rep.NewCount())
	default:
		_, _ = fmt.Fprintf(out, "自動の検証の失敗 %d、基準と違うコマ %d\n", failed, rep.DiffCount())
	}
	_, _ = fmt.Fprintf(out, "確認ページ：%s\n結果：%s\n", page, dir)
	if !rep.Clean() {
		return errors.New("see the page above")
	}
	return nil
}
