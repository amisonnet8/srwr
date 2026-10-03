package main

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

// The VSCode picture of the README: one frame of the demo session drawn the way srwr-view shows it (the colors, the
// layout and the rows of the confirmed design in docs/reference/vscode.md), from the same data the demo session ran with.
// It is not a screenshot of a real VSCode: VSCode cannot be captured by a command here.

type palette struct {
	editor, tabs, tab, activity, side, sideHead, current string
	text, gutter, dim, side2                             string
	keyword, str                                         string
	selectDot, replaceDot, finalDot                      string
	selectRange, replaceRange                            string
}

var vscodeDark = palette{editor: "#1e1e1e", tabs: "#2d2d2d", tab: "#1e1e1e", activity: "#333333", side: "#252526", sideHead: "#2d2d2d", current: "#37373d",
	text: "#d4d4d4", gutter: "#858585", dim: "#9d9d9d", side2: "#cccccc", keyword: "#569cd6", str: "#ce9178",
	selectDot: "#4aa3ff", replaceDot: "#f0883e", finalDot: "#b180d7", selectRange: "#1d3a5c", replaceRange: "#583c27"}

var vscodeLight = palette{editor: "#ffffff", tabs: "#ececec", tab: "#ffffff", activity: "#2c2c2c", side: "#f3f3f3", sideHead: "#ececec", current: "#e4e6f1",
	text: "#333333", gutter: "#237893", dim: "#717171", side2: "#616161", keyword: "#0000ff", str: "#a31515",
	selectDot: "#0b61a4", replaceDot: "#b45f06", finalDot: "#652d90", selectRange: "#cfe3fb", replaceRange: "#fde3c8"}

const (
	codeW   = 8.4 // the width of a character of the code font (14px)
	lineH   = 19
	whyWrap = 66
)

var tokenRe = regexp.MustCompile(`"[^"]*"|\b(?:package|import|func|return|if)\b|[^"\w]+|\w+`)

var keywords = map[string]bool{"package": true, "import": true, "func": true, "return": true, "if": true}

// vscodeFrame draws demo frame `current` (0-based; it must be a select or a replace of demoSteps) in one theme.
func vscodeFrame(theme string, current int) string {
	p := vscodeDark
	if theme == "light" {
		p = vscodeLight
	}
	step := demoSteps[current]
	lines := strings.Split(strings.TrimSuffix(demoSource, "\n"), "\n")
	// The file as it is at that frame: the replaces before it, and this one.
	for i := 0; i <= current; i++ {
		s := demoSteps[i]
		if s.kind == "replace" {
			lines = splice(lines, demoSteps[i-1].start, demoSteps[i-1].end, strings.Split(s.newText, "\n"))
		}
	}
	start, end := step.start, step.end
	if step.kind == "replace" {
		prev := demoSteps[current-1]
		start, end = prev.start, prev.start+len(strings.Split(step.newText, "\n"))-1
	}
	whyBg, rangeBg, tone := "#0b61a4", p.selectRange, "select"
	if step.kind == "replace" {
		whyBg, rangeBg, tone = "#b45f06", p.replaceRange, "replace"
	}
	why := wrapWhy("◆ "+step.why, whyWrap)

	const width, sideX, edX, tabH, statusH = 1000, 40, 290, 35, 22
	height := tabH + (len(lines)+len(why))*lineH + 24 + statusH
	var b strings.Builder
	f := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }
	ui := "'Segoe UI','Hiragino Sans','Noto Sans JP','Meiryo',system-ui,sans-serif"
	mono := "'Cascadia Code','DejaVu Sans Mono','Menlo','Consolas','Noto Sans Mono CJK JP','Meiryo',monospace"
	f(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="srwr-view in VSCode (%s): the %s with its reason line, the operations list and the bottom bar">`, width, height, width, height, theme, tone)
	f(`<rect width="%d" height="%d" fill="%s"/>`, width, height, p.editor)
	f(`<rect width="%d" height="%d" fill="%s"/>`, sideX, height-statusH, p.activity)
	f(`<g transform="translate(8,8)" fill="none" stroke="#ffffff" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect x="2.5" y="6" width="19" height="12" rx="2"/><circle cx="8" cy="12" r="2.5"/><circle cx="16" cy="12" r="2.5"/><path d="M6 18l1.5-3M18 18l-1.5-3"/></g>`)
	f(`<rect x="0" y="4" width="2" height="36" fill="#ffffff"/>`)
	f(`<rect x="%d" width="%d" height="%d" fill="%s"/>`, sideX, edX-sideX, height-statusH, p.side)
	f(`<text x="%d" y="20" fill="%s" font-family="%s" font-size="11">SRWR</text>`, sideX+14, p.side2, ui)
	f(`<rect x="%d" y="30" width="%d" height="22" fill="%s"/>`, sideX, edX-sideX, p.sideHead)
	f(`<text x="%d" y="45" fill="%s" font-family="%s" font-size="11" font-weight="bold">Operations</text>`, sideX+14, p.side2, ui)
	kinds := []string{"select", "replace", "select", "replace", "final"}
	files := []string{"main.go:9-11", "main.go:9-14", "main.go:5-7", "main.go:5-8", "main.go"}
	for i, k := range kinds {
		y := 52 + i*22
		if i == current {
			f(`<rect x="%d" y="%d" width="%d" height="22" fill="%s"/>`, sideX, y, edX-sideX, p.current)
		}
		dot := map[string]string{"select": p.selectDot, "replace": p.replaceDot, "final": p.finalDot}[k]
		f(`<circle cx="%d" cy="%d" r="4.5" fill="%s"/>`, sideX+16, y+11, dot)
		label := fmt.Sprintf("%d  %s  %s", i+1, k, files[i])
		f(`<text x="%d" y="%d" fill="%s" font-family="%s" font-size="12" xml:space="preserve">%s</text>`, sideX+30, y+15, p.text, ui, html.EscapeString(label))
	}
	f(`<rect x="%d" width="%d" height="%d" fill="%s"/>`, edX, width-edX, tabH, p.tabs)
	f(`<rect x="%d" width="90" height="%d" fill="%s"/>`, edX, tabH, p.tab)
	f(`<text x="%d" y="22" fill="%s" font-family="%s" font-size="13" font-style="italic">main.go</text>`, edX+14, p.text, ui)
	y := tabH
	row := func(n int, text string, bg string) {
		if bg != "" {
			f(`<rect x="%d" y="%d" width="%d" height="%d" fill="%s"/>`, edX+60, y, width-edX-60, lineH, bg)
		}
		if n > 0 {
			f(`<text x="%d" y="%d" text-anchor="end" fill="%s" font-family="%s" font-size="14">%d</text>`, edX+44, y+14, p.gutter, mono, n)
		}
		x := float64(edX + 76)
		for _, tok := range tokenRe.FindAllString(text, -1) {
			color := p.text
			switch {
			case strings.HasPrefix(tok, `"`):
				color = p.str
			case keywords[tok]:
				color = p.keyword
			}
			if strings.TrimSpace(tok) != "" {
				f(`<text x="%.1f" y="%d" fill="%s" font-family="%s" font-size="14" xml:space="preserve">%s</text>`, x, y+14, color, mono, html.EscapeString(strings.ReplaceAll(tok, "\t", "    ")))
			}
			x += codeW * float64(len([]rune(strings.ReplaceAll(tok, "\t", "    "))))
		}
		y += lineH
	}
	for i, text := range lines {
		n := i + 1
		if n == start {
			for _, w := range why {
				f(`<rect x="%d" y="%d" width="%d" height="%d" fill="%s"/>`, edX+60, y, width-edX-60, lineH, whyBg)
				f(`<text x="%d" y="%d" fill="#ffffff" font-family="%s" font-size="14" font-weight="bold" xml:space="preserve">%s</text>`, edX+76, y+14, mono, html.EscapeString(w))
				y += lineH
			}
		}
		bg := ""
		if n >= start && n <= end {
			bg = rangeBg
		}
		row(n, text, bg)
	}
	f(`<rect y="%d" width="%d" height="%d" fill="#007acc"/>`, height-statusH, width, statusH)
	dimBack := ` fill-opacity="1"`
	f(`<text x="10" y="%d" fill="#ffffff"%s font-family="%s" font-size="12">‹ Back</text>`, height-7, dimBack, ui)
	f(`<text x="72" y="%d" fill="#ffffff" font-family="%s" font-size="12">Forward ›</text>`, height-7, ui)
	f(`<text x="150" y="%d" fill="#ffffff" font-family="%s" font-size="12">%d/5</text>`, height-7, ui, current+1)
	f(`<text x="190" y="%d" fill="#ffffff" font-family="%s" font-size="12">×</text>`, height-7, ui)
	b.WriteString("</svg>\n")
	return b.String()
}

// splice replaces the 1-based lines start..end of lines.
func splice(lines []string, start, end int, with []string) []string {
	out := append([]string{}, lines[:start-1]...)
	out = append(out, with...)
	return append(out, lines[end:]...)
}

// wrapWhy cuts text into rows of at most width characters at spaces; the next rows are indented under the text.
func wrapWhy(text string, width int) []string {
	var rows []string
	cur := ""
	for _, w := range strings.Fields(text) {
		if cur != "" && len([]rune(cur))+1+len([]rune(w)) > width {
			rows = append(rows, cur)
			cur = "  " + w
			continue
		}
		if cur == "" {
			cur = w
		} else {
			cur += " " + w
		}
	}
	if cur != "" {
		rows = append(rows, cur)
	}
	return rows
}
