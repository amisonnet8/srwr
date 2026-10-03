package uicheck

import (
	"fmt"
	"html"
	"strings"
)

// ShotSVG draws what the extension showed for one frame in a simple VSCode-like picture (not a screenshot of VSCode, see
// handoff/design/DESIGN.md 12章): the list on the left, the editors with the lines that are painted, the bar at the bottom.
// The colors and the text are the ones the extension gave; the shape (fonts, tabs, icons) is not VSCode's.
func ShotSVG(s Shot, title string) string {
	const (
		w, h   = 1280, 520
		listW  = 330
		lineH  = 19
		shown  = 22
		bg     = "#1e1e1e"
		fg     = "#d4d4d4"
		dimFG  = "#858585"
		sansFB = "'Segoe UI','Hiragino Sans','Noto Sans JP','Meiryo',system-ui,sans-serif"
		mono   = "'DejaVu Sans Mono','Menlo','Consolas','Noto Sans Mono CJK JP','Noto Sans JP','Meiryo',monospace"
	)
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-label="%s">`+"\n", w, h, w, h, html.EscapeString(title))
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="%s"/>`+"\n", w, h, bg)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#252526"/>`+"\n", listW, h-24)
	text := func(x, y float64, fill, family string, size int, s string) {
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" fill="%s" font-family="%s" font-size="%d" xml:space="preserve">%s</text>`+"\n", x, y, fill, family, size, html.EscapeString(s))
	}
	// the list
	tree, _ := s["tree"].([]any)
	current := -1
	if f, ok := s["frame"].(float64); ok {
		current = int(f)
	}
	for i, it := range tree {
		m, _ := it.(map[string]any)
		y := 22 + i*lineH
		if y > h-40 {
			break
		}
		if i == current {
			fmt.Fprintf(&b, `<rect x="0" y="%d" width="%d" height="%d" fill="#37373d"/>`+"\n", y-14, listW, lineH)
		}
		dot := map[string]string{"select": "#3794ff", "replace": "#d18616", "external": "#b180d7", "final": "#b180d7"}[fmt.Sprint(m["kind"])]
		if dot == "" {
			dot = dimFG
		}
		fmt.Fprintf(&b, `<circle cx="12" cy="%d" r="5" fill="%s"/>`+"\n", y-4, dot)
		text(24, float64(y), fg, sansFB, 12, fmt.Sprint(m["label"]))
	}
	// the editors
	tabs, _ := s["tabs"].([]any)
	if len(tabs) == 0 {
		text(listW+20, 40, dimFG, sansFB, 13, "（開いているエディタはありません）")
	}
	colW := (w - listW) / max(len(tabs), 1)
	for ti, t := range tabs {
		tab, _ := t.(map[string]any)
		x0 := listW + ti*colW
		uri := fmt.Sprint(tab["uri"])
		if i := strings.LastIndex(uri, "/"); i >= 0 {
			uri = uri[i+1:]
		}
		fmt.Fprintf(&b, `<rect x="%d" y="0" width="%d" height="26" fill="#2d2d2d"/>`+"\n", x0, colW)
		text(float64(x0+10), 17, fg, sansFB, 12, uri)
		lines := strings.Split(fmt.Sprint(tab["text"]), "\n")
		painted := paintedLines(tab["decorations"])
		labels := lineLabels(tab["decorations"])
		reveal := 1
		if r, ok := tab["reveal"].(float64); ok {
			reveal = int(r)
		}
		start := max(reveal-1-6, 0)
		for i := 0; i < shown && start+i < len(lines); i++ {
			ln := start + i
			y := 26 + i*lineH
			if p, ok := painted[ln]; ok {
				fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" fill="%s"/>`+"\n", x0, y, colW, lineH, p.bg)
			}
			number := fmt.Sprintf("%3d", ln+1)
			if l, ok := labels[ln]; ok {
				number = l // the extension's own numbers: the why rows have none
			}
			text(float64(x0+6), float64(y+14), dimFG, mono, 12, number)
			col := fg
			if p, ok := painted[ln]; ok && p.fg != "" {
				col = p.fg
			}
			text(float64(x0+40), float64(y+14), col, mono, 12, strings.ReplaceAll(truncate(lines[ln], 80), "\t", "    "))
		}
	}
	// the bar
	fmt.Fprintf(&b, `<rect y="%d" width="%d" height="24" fill="#007acc"/>`+"\n", h-24, w)
	var bar []string
	if st, ok := s["status"].([]any); ok {
		for _, it := range st {
			m, _ := it.(map[string]any)
			bar = append(bar, fmt.Sprint(m["text"]))
		}
	}
	text(10, float64(h-8), "#ffffff", sansFB, 12, strings.Join(bar, "    "))
	b.WriteString("</svg>\n")
	return b.String()
}

type paint struct{ bg, fg string }

// paintedLines is the whole-line decorations of an editor: line (0-based) to the colors.
func paintedLines(v any) map[int]paint {
	out := map[int]paint{}
	list, _ := v.([]any)
	for _, d := range list {
		m, _ := d.(map[string]any)
		opts, _ := m["opts"].(map[string]any)
		bg, _ := opts["backgroundColor"].(string)
		if bg == "" {
			continue
		}
		fg, _ := opts["color"].(string)
		ranges, _ := m["ranges"].([]any)
		for _, r := range ranges {
			rm, _ := r.(map[string]any)
			if line, ok := rm["line"].(float64); ok {
				out[int(line)] = paint{bg, fg}
			}
		}
	}
	return out
}

// lineLabels are the line numbers the extension draws itself (a decoration with a text before the line): line (0-based) to
// the text, without the padding. A line with an empty text has no number. Without such decorations, the editor's own numbers
// count and the map is empty.
func lineLabels(v any) map[int]string {
	out := map[int]string{}
	list, _ := v.([]any)
	for _, d := range list {
		m, _ := d.(map[string]any)
		ranges, _ := m["ranges"].([]any)
		for _, r := range ranges {
			rm, _ := r.(map[string]any)
			before, ok := rm["before"].(string)
			line, lok := rm["line"].(float64)
			if ok && lok {
				out[int(line)] = fmt.Sprintf("%3s", strings.TrimSpace(strings.ReplaceAll(before, "\u00a0", " ")))
			}
		}
	}
	return out
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
