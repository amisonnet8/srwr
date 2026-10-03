package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// vimGuide is what a person does in Vim to look at each tape: numbered steps, every one with the key to press and what
// to look at (.claude/rules/working-with-human.md 3章).
var vimGuide = map[string][]string{
	"why-basic": {
		"左に操作一覧（7コマ）、右にコマが開きます。1コマ目は text.go の select です",
		"`]]` を押して進み、5コマ目（replace）まで進みます。`[[` で戻れます",
		"見る：select は青の理由の行＋薄い青の範囲、replace は橙。理由の行の下の行が範囲です。行番号は理由の行だけ空白です",
		"`:set background=light` と打って Enter。色が light 用（薄い色）に変わっても、読めるか見ます。戻すには `:set background=dark`",
		"左の一覧で `<CR>`（Enter）を押すと、その行のコマに移ります",
		"`q` で閉じます",
	},
	"external": {
		"左に操作一覧（11コマ）、右にコマが開きます",
		"`]]` を6回押して、6コマ目（外部変更）まで進みます",
		"見る：左右2つのウィンドウに分かれ、左（前）は青、右（後）は橙で、変わった行だけが塗られているか。左下に「前 ⚠ srwrの外で変更：stats.go」。`]c` で次の変更へ飛べます",
		"`]]` を押して7コマ目へ。右のウィンドウが消えて、1つの画面に戻るか見ます",
		"`]]` を押して10・11コマ目（録画後）まで進み、同じ形の差分（見出しは「録画のあとで変更」）が出るか見ます",
		"`:set background=light` で light でも差分の青と橙が読めるか見ます。`q` で閉じます",
	},
	"no-why": {
		"左に操作一覧（12コマ）、右にコマが開きます。理由が無いので、理由の行は出ません",
		"`]]` で進みながら、範囲だけが薄い青（select）・薄い橙（replace）で塗られ、行番号が Vim 標準のままか見ます",
		"11・12コマ目（録画後）は左右の差分になります。`q` で閉じます",
	},
	"live": {
		"Vim が開いて約5秒後から、3秒ごとに1コマずつ追記されます（全部で6コマ、約20秒）。それまでは「ライブ視聴中（AI の操作を待っています）」と出ます",
		"見る（追っている間）：コマが届くたびに画面が最新のコマに替わり、ステータス行に「● LIVE」が出続けるか。ちらつかないか",
		"追記が続いている間に `[[` を押します。画面は動かず、ステータス行に橙の背景で「L：LIVE に戻る（新着 N）」が出て、N が増えるか見ます（100桁ほどの幅でも読めます）",
		"`L` を押します。最新のコマに移り、「● LIVE」に戻るか見ます",
		"追記が終わったら `q` で閉じます。見逃したら、`q` で閉じて、もう一度 `qsoku ui-open vim live`",
	},
}

func openVim(root, name string, out io.Writer) error {
	bin := filepath.Join(root, "bin", "srwr")
	fixture := filepath.Join(root, "extension", "test", "fixtures", "ui-check")
	base := os.Getenv("SRWR_UI_OPEN_DIR")
	if base == "" {
		base = filepath.Join(os.TempDir(), "srwr-ui-open")
	}
	workspace := filepath.Join(base, "vim-"+name)
	if err := prepareWorkspace(fixture, workspace, bin, name == "live"); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "作業場：%s\n\n【%s】手順\n", workspace, name)
	for i, step := range vimGuide[name] {
		_, _ = fmt.Fprintf(out, "%d. %s\n", i+1, step)
	}
	_, _ = fmt.Fprint(out, "\n読んだら Enter を押してください（Vim が開きます）: ")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')

	args := []string{"view"}
	if name == "live" {
		args = append(args, "--live")
	} else {
		args = append(args, tapes[name].ID)
	}
	cmd := exec.Command(bin, args...) //nolint:gosec // the binary this repository built
	cmd.Dir = workspace
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if name == "live" {
		return runLiveVim(cmd, workspace, fixture)
	}
	return cmd.Run()
}

// runLiveVim writes the start of the live tape, starts Vim, and a few seconds later (when Vim has opened the live view)
// feeds the rest of the tape into the workspace.
func runLiveVim(cmd *exec.Cmd, workspace, fixture string) error {
	if err := os.MkdirAll(filepath.Join(workspace, ".srwr", "tapes"), 0o750); err != nil { //nolint:gosec // the workspace of this tool
		return err
	}
	started := make(chan struct{})
	fed := make(chan error, 1)
	go func() {
		fed <- feedLive(workspace, fixture, func() {
			close(started) // the first frames are in the tape; Vim may start
			time.Sleep(5 * time.Second)
		}, 3*time.Second, io.Discard)
	}()
	select {
	case <-started:
	case err := <-fed:
		return err
	}
	runErr := cmd.Run()
	select {
	case err := <-fed:
		if err != nil && runErr == nil {
			return err
		}
	default:
	}
	return runErr
}
