package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// dist-try is the human check of R13 (`qsoku dist-try`): what is handed to the users, and nothing else, is put to use. The srwr
// of this machine is taken out of its archive in dist/ (not bin/srwr) and the .vsix of dist/ is installed into VSCode, which
// opens a fixed tape through the setting srwr.path; then Vim opens the same tape with that srwr. The checks that a machine can do
// are marked on the page; what a person has to see is written there.

// distArchive finds the archive of this machine in dist/.
func distArchive(dist string) (string, error) {
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	matches, err := filepath.Glob(filepath.Join(dist, "srwr_*_"+runtime.GOOS+"_"+runtime.GOARCH+ext))
	if err != nil || len(matches) != 1 {
		return "", fmt.Errorf("no archive for %s/%s in %s (found %v); run qsoku dist first", runtime.GOOS, runtime.GOARCH, dist, matches)
	}
	return matches[0], nil
}

// extractExe writes the srwr executable out of a .tar.gz or .zip into dir and returns its path.
func extractExe(archive, dir string) (string, error) {
	b, err := os.ReadFile(archive) //nolint:gosec // a file of dist/
	if err != nil {
		return "", err
	}
	name := "srwr"
	if runtime.GOOS == "windows" {
		name = "srwr.exe"
	}
	var data []byte
	if strings.HasSuffix(archive, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			return "", err
		}
		for _, f := range zr.File {
			if f.Name == name {
				rc, err := f.Open()
				if err != nil {
					return "", err
				}
				data, err = io.ReadAll(rc)
				_ = rc.Close()
				if err != nil {
					return "", err
				}
			}
		}
	} else {
		gz, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return "", err
		}
		tr := tar.NewReader(gz)
		for {
			h, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return "", err
			}
			if h.Name == name {
				if data, err = io.ReadAll(tr); err != nil {
					return "", err
				}
			}
		}
	}
	if len(data) == 0 {
		return "", fmt.Errorf("%s has no %s", filepath.Base(archive), name)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	exe := filepath.Join(dir, name)
	return exe, os.WriteFile(exe, data, 0o755) //nolint:gosec // it is an executable
}

// distTryReport marks what a machine can check. exe is the srwr out of dist/, workspace the fixed workspace.
func distTryReport(root, exe, workspace, vsix string) (lines []string, ok bool) {
	ok = true
	check := func(cond bool, format string, a ...any) {
		mark := "○"
		if !cond {
			mark, ok = "×", false
		}
		lines = append(lines, mark+" "+fmt.Sprintf(format, a...))
	}
	version, err := exec.Command(exe, "--version").Output()
	check(err == nil && strings.HasPrefix(string(version), "srwr "), "dist/ の srwr が動き、版を言う：%s", strings.TrimSpace(string(version)))
	list := exec.Command(exe, "tapes", "--root", workspace) //nolint:gosec // see above
	listed, err := list.Output()
	missing := []string{}
	for _, name := range []string{"why-basic", "external", "no-why"} {
		if !strings.Contains(string(listed), tapes[name].ID) {
			missing = append(missing, tapes[name].ID)
		}
	}
	check(err == nil && len(missing) == 0, "dist/ の srwr が、固定テープ3本を一覧できる（srwr tapes）%s", strings.Join(missing, " "))
	var checked bytes.Buffer
	pc := exec.Command("go", "run", "./tools/dist", "publish-check", vsix) //nolint:gosec // fixed arguments
	pc.Dir, pc.Stdout, pc.Stderr = root, &checked, &checked
	check(pc.Run() == nil, ".vsix の中身が正しい：srwr のバイナリが入っていない、README の画像が公開用の URL、アイコン・翻訳がある")
	return lines, ok
}

func renderDistTryPage(lines []string, vsix, exe string, ok bool) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="ja"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>配布物の確認</title><style>` + pageCSS + `</style></head><body>` + "\n")
	if ok {
		b.WriteString(`<div class="big ok">機械で確かめられることは全部 ○ でした</div><p>下の「人間が見ること」を上から順にやって、OK／NG をチャットで答えてください。</p>` + "\n")
	} else {
		b.WriteString(`<div class="big ng">× があります</div><p>この画面のまま、チャットで「NG」と伝えてください（AI が原因を調べます）。</p>` + "\n")
	}
	b.WriteString("<h2>機械の検査</h2><table><tr><th>結果</th><th>確かめたこと</th></tr>\n")
	for _, l := range lines {
		cls := "ok"
		if strings.HasPrefix(l, "×") {
			cls = "ng"
		}
		mark, text, _ := strings.Cut(l, " ")
		fmt.Fprintf(&b, `<tr class="%s"><td>%s</td><td>%s</td></tr>`+"\n", cls, html.EscapeString(mark), html.EscapeString(text))
	}
	b.WriteString("</table>\n")
	fmt.Fprintf(&b, "<h2>使ったもの（dist/ のものだけ。bin/srwr は使っていません）</h2><ul><li>srwr：<code>%s</code></li><li>.vsix：<code>%s</code></li></ul>\n", html.EscapeString(exe), html.EscapeString(vsix))
	b.WriteString(`<h2>人間が見ること</h2>
<h3>A. VSCode（コマンドを動かすと、新しい窓で開いています）</h3>
<ol>
<li>左端の「srwr」（カセットの絵）をクリックし、「Operations」（日本語の表示言語なら「操作一覧」）の「Open a tape」（「テープを開く」）を押して、一番上の <code>2026-09-30 …</code> を選ぶ。</li>
<li>見る：左の一覧に 1〜7 の行（青の丸が4つ、橙の丸が3つ）が出て、右に <code>text.go</code> と青い理由の行が出る。下のバーに「‹ Back　Forward ›　1/7」が出る。</li>
<li>「Forward」を5回押す。見る：6/7 のコマで橙の理由の行が出て、その下の行が薄い橙で塗られる。</li>
<li>OK の基準：上の3つが出る。「srwr を起動できません」「バージョンが合っていません」などのエラーが出ない（この窓は、設定 srwr.path で dist/ の srwr を指しています）。</li>
</ol>
<h3>B. Vim（コマンドのウィンドウで Enter を押すと開きます）</h3>
<ol>
<li>日本語入力（IME）を切る。Vim が開いたら、左に操作一覧（7行）、右にコマが出る。</li>
<li><code>]]</code> を5回押す。見る：6/7 で橙の理由の行が出て、ステータス行に「srwr  6/7」が出る。</li>
<li><code>q</code> で閉じる。</li>
<li>OK の基準：上の2つが出る。「Vim が見つかりません」などのエラーが出ない（この Vim は、dist/ の srwr に埋め込まれたスクリプトで動いています）。</li>
</ol>
<h3>C. 終わったら</h3>
<p>拡張を戻したいときは、VSCode の拡張の一覧で srwr-view を「アンインストール」します（この確認では、dist/ の .vsix を入れたままにしています）。</p>
</body></html>
`)
	return b.String()
}

func runDistTry(root string, in io.Reader, out io.Writer) error {
	dist := filepath.Join(root, "dist")
	archive, err := distArchive(dist)
	if err != nil {
		return err
	}
	vsixes, _ := filepath.Glob(filepath.Join(dist, "*.vsix"))
	sort.Strings(vsixes)
	if len(vsixes) == 0 {
		return fmt.Errorf("no .vsix in %s; run qsoku dist first", dist)
	}
	vsix := vsixes[len(vsixes)-1]

	base := filepath.Join(os.TempDir(), "srwr-dist-try")
	exe, err := extractExe(archive, filepath.Join(base, "bin"))
	if err != nil {
		return err
	}
	workspace := filepath.Join(base, "workspace")
	// srwr.path of the workspace is the srwr out of dist/: the setting of the extension is part of what is checked.
	if err := prepareWorkspace(filepath.Join(root, "extension", "test", "fixtures", "ui-check"), workspace, exe, false); err != nil {
		return err
	}
	lines, ok := distTryReport(root, exe, workspace, vsix)
	page := filepath.Join(root, "ui-check-result", "dist-try", "index.html")
	if err := os.MkdirAll(filepath.Dir(page), 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(page, []byte(renderDistTryPage(lines, vsix, exe, ok)), 0o600); err != nil {
		return err
	}
	for _, l := range lines {
		_, _ = fmt.Fprintln(out, l)
	}
	_, _ = fmt.Fprintf(out, "\n確認ページ：%s\n", page)
	if !ok {
		return errors.New("a check failed; the page says which")
	}
	if _, err := exec.LookPath("code"); err != nil {
		return fmt.Errorf("the code command was not found. Install %s into VSCode by hand and open %s", vsix, workspace)
	}
	if err := checkCodeTerminal(os.Getenv("VSCODE_IPC_HOOK_CLI")); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, "▶ dist/ の .vsix を VSCode に入れる")
	if err := command(root, out, "code", "--install-extension", vsix, "--force"); err != nil {
		return fmt.Errorf("installing the extension: %w", err)
	}
	_, _ = fmt.Fprintf(out, "▶ 作業場を VSCode の新しい窓で開く：%s\n", workspace)
	if err := command(root, out, "code", "-n", workspace); err != nil {
		return fmt.Errorf("opening VSCode: %w", err)
	}
	_, _ = fmt.Fprint(out, "\nVSCode は、確認ページの「A」を上からやってください。終わったら、ここで Enter を押すと Vim が開きます（ページの「B」）: ")
	_, _ = bufio.NewReader(in).ReadString('\n')
	cmd := exec.Command(exe, "view", tapes["why-basic"].ID) //nolint:gosec // the binary taken out of dist/
	cmd.Dir = workspace
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), vimEnv(os.Getenv("SRWR_LANG"), false)...)
	return cmd.Run()
}
