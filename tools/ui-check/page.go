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
			if c.Status == statusSame || (c.Status == statusNew && set.title != "Vim") {
				continue // the extension has no pictures; a new screen of it is looked at in a real VSCode (the steps below)
			}
			if !shown {
				b.WriteString("<h2>基準と違うコマ・新しいコマ</h2>\n")
				shown = true
			}
			writeCapture(&b, set.title, c)
		}
	}
	if !shown {
		if r.NewCount() > 0 {
			b.WriteString(`<p>画像で見るものはありません。下の「人間が見ること」のとおりにしてください。</p>` + "\n")
		} else {
			b.WriteString(`<p>どの画面も基準と同じです。</p>` + "\n")
		}
	}

	b.WriteString(`<h2>人間が見ること</h2>`)
	steps := lookSteps(r)
	if len(steps) == 0 {
		b.WriteString("<p>変更が無いので、確認は要りません。</p>\n")
	} else {
		b.WriteString("<ol>\n")
		for _, st := range steps {
			fmt.Fprintf(&b, "<li>%s</li>\n", st)
		}
		b.WriteString("</ol>\n<p>見終わったら、OK か NG かをチャットで答えてください（NG のときは、どの画像のどこかを一言）。OK なら、AI が今の画面を次の基準にします。</p>\n")
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

// tapeOf is the name `qsoku ui-open` knows for the capture of that name.
func tapeOf(full string) string {
	name := strings.Fields(full)[0] // "replay-why-basic en dark" and "all_basic en" are named by their first word
	switch {
	case strings.Contains(name, "long-why") || name == "long_why":
		return "long-why"
	case strings.Contains(name, "why-basic") || name == "all_basic":
		return "why-basic"
	case strings.Contains(name, "no-why") || name == "all_nowhy":
		return "no-why"
	case strings.Contains(name, "external") || name == "all_ext":
		return "external"
	case strings.HasPrefix(name, "live"):
		return "live"
	}
	return name
}

// lookSteps are what a person does, in order, with the exact words; nothing is left to think over. Each is HTML.
func lookSteps(r *Report) []string {
	var steps []string
	for _, c := range r.Vim {
		if c.Status != statusNew && c.Status != statusDiff {
			continue
		}
		theme := "dark"
		if strings.HasSuffix(c.Name, " light") {
			theme = "light"
		}
		if c.Status == statusNew {
			steps = append(steps, fmt.Sprintf("上の画像「Vim・%s」を見る：%s", html.EscapeString(c.Name), html.EscapeString(tapes[tapeOf(c.Name)].Look)))
		} else {
			steps = append(steps, fmt.Sprintf("上の画像「Vim・%s」を見る：右の「今」の赤枠の部分が、意図した変更か（色が読めるかも見る。%s の背景）", html.EscapeString(c.Name), theme))
		}
	}
	for _, c := range r.VSCode {
		if c.Status != statusNew && c.Status != statusDiff {
			continue
		}
		// A real VSCode is looked at for the two tapes that show every part of the screen (the diff frames, the list, the picker, the
		// bar; the live bar). The other captures are the same texts, which the machine compares.
		if t := tapeOf(c.Name); t != "external" && t != "live" {
			continue
		}
		steps = append(steps, fmt.Sprintf("実物の VSCode で「VSCode・%s」を見る。手順：\n%s", html.EscapeString(c.Name), vscodeSteps(tapeOf(c.Name), c.Name)))
	}
	return steps
}

// vscodeSteps are the steps to look at a tape in a real VSCode, written out in full (HTML, an ordered list).
func vscodeSteps(tape, _ string) string {
	t := tapes[tape]
	look := html.EscapeString(t.Look)
	steps := []string{
		fmt.Sprintf("日本語入力を切り、ターミナルで <code>qsoku ui-open vscode %s</code> と打って Enter を押す。新しい VSCode が開く（数十秒かかることがある）", tape),
		fmt.Sprintf("開いた VSCode で、左端のカセットのアイコンを押し、「Open a tape」を選ぶ（VSCode の表示は英語）。出てきた一覧から「%s」を選ぶ", html.EscapeString(t.Started)),
		"1 コマ目が開く。見る：" + look,
		"下のバーの「Forward」を押して次のコマへ進み、同じように見る。「Back」で戻れる",
	}
	if tape == "live" {
		steps = []string{
			fmt.Sprintf("日本語入力を切り、ターミナルで <code>qsoku ui-open vscode %s</code> と打って Enter を押す。新しい VSCode が開く", tape),
			"開いた VSCode で、左端のカセットのアイコンを押し、「Start live view」を押す（VSCode の表示は英語）",
			"ターミナルに戻り、Enter を押す。3 秒ごとに 1 コマずつ追記される",
			"見る：" + look,
		}
	}
	var b strings.Builder
	b.WriteString("<ol>\n")
	for _, s := range steps {
		fmt.Fprintf(&b, "<li>%s</li>\n", s)
	}
	b.WriteString("</ol>\n")
	return b.String()
}
