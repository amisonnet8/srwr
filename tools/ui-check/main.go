// Command ui-check prepares what a person needs to look at the UI by eye (docs: .claude/rules/working-with-human.md 3章).
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
	if len(args) == 3 && args[0] == "vim-baseline" {
		return makeVimBaseline(args[1], args[2])
	}
	if len(args) != 3 || args[0] != "open" || (args[1] != "vscode" && args[1] != "vim") {
		return errors.New("usage: ui-check open <vscode|vim> <why-basic|external|no-why|live>")
	}
	name := args[2]
	if _, ok := tapes[name]; !ok && name != "live" {
		return fmt.Errorf("unknown tape %q: why-basic, external, no-why or live", name)
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
