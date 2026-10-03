package main

import (
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// writeReadmePage makes ui-check-result/readme/index.html: the pictures of the README, dark and light, for a person to
// look at. The pictures are put into the page itself, so that it opens anywhere.
func writeReadmePage(root string, out io.Writer) error {
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="ja"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>README の画像</title>
<style>body{font-family:system-ui,sans-serif;margin:24px;max-width:1300px}img{max-width:100%;border:1px solid #8884}h2{margin-top:32px}.pair{display:grid;gap:12px}.on-dark{background:#0d1117;padding:12px}.on-light{background:#fff;padding:12px}</style></head><body>
<h1>README の画像</h1>
<p>見るもの：①バナー、②Vim のデモ（コマ送りで動く。暗い・明るい）、③VSCode の絵（暗い・明るい）。OK／NG とひとことをチャットで答えてください。</p>
`)
	images := filepath.Join(root, "docs", "images")
	for _, it := range []struct{ title, file string }{
		{"① バナー（暗い背景・明るい背景の両方で読めること）", "banner.svg"},
		{"② Vim のデモ（dark）：5コマが順に切り替わり、最後にもう一度最初へ戻る", "demo-vim_dark.svg"},
		{"② Vim のデモ（light）", "demo-vim_light.svg"},
		{"③ VSCode の絵（dark）：橙の理由の行、範囲、操作一覧、下のバー", "vscode_dark.svg"},
		{"③ VSCode の絵（light）", "vscode_light.svg"},
	} {
		data, err := os.ReadFile(filepath.Join(images, it.file)) //nolint:gosec // a picture this tool just wrote
		if err != nil {
			return err
		}
		bg := "on-light"
		if strings.Contains(it.file, "dark") || it.file == "banner.svg" {
			bg = "on-dark"
		}
		fmt.Fprintf(&b, "<h2>%s</h2><div class=\"%s\">%s</div>\n", html.EscapeString(it.title), bg, string(data))
		if it.file == "banner.svg" {
			fmt.Fprintf(&b, "<div class=\"on-light\">%s</div>\n", string(data))
		}
	}
	b.WriteString("</body></html>\n")
	dir := filepath.Join(root, "ui-check-result", "readme")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	page := filepath.Join(dir, "index.html")
	if err := os.WriteFile(page, []byte(b.String()), 0o600); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(out, "page:", page)
	return nil
}
