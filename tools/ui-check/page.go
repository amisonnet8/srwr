package main

import (
	"fmt"
	"html"
	"strings"
)

// maxFramesShown is how many differing frames of one capture the page draws (the rest are counted).
const maxFramesShown = 4

const pageCSS = `body{margin:16px auto;max-width:1500px;font:15px/1.7 sans-serif;padding:0 16px;color:#222;background:#fff}
.big{font-size:32px;font-weight:bold;margin:8px 0}.ok{color:#1a7f37}.ng{color:#c62828}.new{color:#b45f06}
.meta{color:#555;font-size:13px}table{border-collapse:collapse;margin:6px 0}td,th{border:1px solid #bbb;padding:3px 10px;text-align:left;vertical-align:top}
tr.ok td:last-child{color:#1a7f37}tr.ng td{background:#fdecea;color:#c62828}
.pair{display:flex;gap:12px;flex-wrap:wrap}.pair figure{margin:0;width:48%;min-width:300px}.pair svg{width:100%;height:auto;border:1px solid #888;display:block}
figcaption{font-size:13px;color:#555}.why{background:#fff8e1;padding:6px 6px 6px 28px}.note{background:#eef6ff;border-left:4px solid #0b61a4;padding:8px 14px}
h2{margin:22px 0 4px}h3{margin:14px 0 2px}h4{margin:10px 0 2px}code{background:#eee;padding:0 4px}`

// renderPage makes the page a person looks at (the UI gate of R6, dev/review/R6): the summary first, then the checks, then
// the frames that differ (left the baseline, right now, the cells that differ in a red frame), then what a person judges.
func renderPage(r *Report) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="ja"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>UI 確認</title><style>` + pageCSS + `</style></head><body>` + "\n")
	failed := 0
	for _, c := range r.Checks {
		if !c.OK {
			failed++
		}
	}
	switch {
	case failed > 0:
		fmt.Fprintf(&b, `<div class="big ng">自動の検証が %d 件失敗</div>`, failed)
	case len(r.Problems) > 0:
		b.WriteString(`<div class="big ng">確認で問題が見つかった</div>`)
	case r.DiffCount() > 0 || hasError(r):
		fmt.Fprintf(&b, `<div class="big ng">基準と違うコマ %d</div>`, r.DiffCount())
	case r.NewCount() > 0:
		fmt.Fprintf(&b, `<div class="big new">新しいコマ（基準なし）の画面 %d</div>`, r.NewCount())
	default:
		b.WriteString(`<div class="big ok">違いなし</div>`)
	}
	fmt.Fprintf(&b, `<p class="meta">版 %s／%s／`, html.EscapeString(r.Version), html.EscapeString(r.Time))
	if r.LiveOnly {
		b.WriteString("ライブだけ（自動の検証は動かしていません）")
	} else {
		fmt.Fprintf(&b, "自動の検証 %d/%d 通過", len(r.Checks)-failed, len(r.Checks))
	}
	fmt.Fprintf(&b, "／基準と違うコマ %d／新しい画面 %d</p>\n", r.DiffCount(), r.NewCount())

	if len(r.Checks) > 0 {
		b.WriteString("<h2>自動の検証</h2><table><tr><th>項目</th><th>確かめること</th><th>結果</th></tr>\n")
		for _, c := range r.Checks {
			cls, text := "ok", "通過"
			if !c.OK {
				cls, text = "ng", "失敗："+strings.Join(c.Notes, "；")
			}
			fmt.Fprintf(&b, `<tr class="%s"><td>%s</td><td>%s</td><td>%s</td></tr>`+"\n", cls, html.EscapeString(c.Name), html.EscapeString(c.What), html.EscapeString(text))
		}
		b.WriteString("</table>\n")
	}
	if len(r.Problems) > 0 {
		b.WriteString(`<h2>見つかった問題</h2><ul class="why">`)
		for _, p := range r.Problems {
			fmt.Fprintf(&b, "<li>%s</li>", html.EscapeString(p))
		}
		b.WriteString("</ul>\n")
	}

	var shown bool
	for _, set := range []struct {
		title string
		res   []CaptureResult
	}{{"Vim", r.Vim}, {"VSCode（拡張）", r.VSCode}} {
		for _, c := range set.res {
			if c.Status == statusSame {
				continue
			}
			if !shown {
				b.WriteString("<h2>基準と違うコマ・新しいコマ</h2>\n")
				shown = true
			}
			writeCapture(&b, set.title, c)
		}
	}
	if !shown {
		b.WriteString(`<p>どの画面も基準と同じです。</p>` + "\n")
	}

	b.WriteString(`<h2>人間が判断すること</h2>`)
	if r.Clean() && r.NewCount() == 0 {
		b.WriteString("<p>変更が無いので、確認は要りません。</p>\n")
	} else {
		b.WriteString("<ol><li>色：青（select）と橙（replace）が見分けられ、理由の行と範囲が読めるか（dark と light）</li><li>差分：左（青）と右（橙）で、変わった行だけが塗られ、見やすいか</li><li>変えたところが、意図どおりに見えるか</li></ol>\n")
		b.WriteString("<p>実物で見る：<code>qsoku ui-open vim why-basic</code>、<code>qsoku ui-open vscode why-basic</code>（ほかは <code>external</code>・<code>no-why</code>・<code>live</code>）</p>\n")
		b.WriteString("<p class=\"meta\">OK／NG はチャットで答えてください。AI が <code>qsoku ui-accept</code> で記録し、OK なら今の画面が次の基準になります。</p>\n")
	}
	b.WriteString("</body></html>\n")
	return b.String()
}

func hasError(r *Report) bool {
	for _, c := range append(append([]CaptureResult{}, r.Vim...), r.VSCode...) {
		if c.Status == statusError {
			return true
		}
	}
	return false
}

func writeCapture(b *strings.Builder, set string, c CaptureResult) {
	switch c.Status {
	case statusError:
		fmt.Fprintf(b, "<h3>%s・%s：取れなかった</h3><pre>%s</pre>\n", html.EscapeString(set), html.EscapeString(c.Name), html.EscapeString(c.Error))
		return
	case statusNew:
		fmt.Fprintf(b, `<h3 class="new">%s・%s：新しい画面（基準がありません。OK なら今の画面が基準になります）</h3>`+"\n", html.EscapeString(set), html.EscapeString(c.Name))
	default:
		fmt.Fprintf(b, `<h3 class="ng">%s・%s：基準と違う</h3>`+"\n", html.EscapeString(set), html.EscapeString(c.Name))
	}
	if c.Status == statusNew && len(c.Frames) > 0 && c.Frames[0].Got == "" {
		// No picture of the extension: what it shows is compared as text, and how it looks is judged in a real VSCode.
		fmt.Fprintf(b, "<p>%d コマ。見た目は実物で見てください：<code>qsoku ui-open vscode …</code></p>\n", len(c.Frames))
		return
	}
	for i, f := range c.Frames {
		if i >= maxFramesShown {
			fmt.Fprintf(b, "<p class=\"meta\">ほか %d コマ（result.json に一覧があります）</p>\n", len(c.Frames)-maxFramesShown)
			break
		}
		fmt.Fprintf(b, "<h4>%d コマ目（%s）</h4>\n", f.Index, html.EscapeString(f.Label))
		if len(f.Diffs) > 0 {
			b.WriteString(`<ul class="why">`)
			for _, d := range f.Diffs {
				fmt.Fprintf(b, "<li>%s</li>", html.EscapeString(d))
			}
			b.WriteString("</ul>\n")
		}
		b.WriteString(`<div class="pair">`)
		if f.Want != "" {
			fmt.Fprintf(b, "<figure><figcaption>基準（前回 OK）</figcaption>%s</figure>", f.Want)
		}
		if f.Got != "" {
			caption := "今（赤枠が違うところ）"
			if c.Status == statusNew {
				caption = "今"
			}
			fmt.Fprintf(b, "<figure><figcaption>%s</figcaption>%s</figure>", caption, f.Got)
		}
		b.WriteString("</div>\n")
	}
}
