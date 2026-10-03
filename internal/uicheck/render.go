package uicheck

import (
	"fmt"
	"html"
	"sort"
	"strings"
)

// GridOf makes a screen from a reduced Frame (text, backgrounds, the foregrounds that were checked). A cell the frame does not
// color has the Normal background and the plain foreground fg.
func GridOf(f Frame, cols, rows int, normal, fg string) *Grid {
	g := NewGrid(cols, rows, normal)
	for r, line := range f.Text {
		if r >= rows {
			break
		}
		g.Put(r, 0, line, fg, normal)
	}
	for _, run := range f.BG {
		for c := run.Col; c < run.Col+run.Len && c < cols; c++ {
			g.Cells[run.Row][c].BG = run.Color
		}
	}
	for _, run := range f.FG {
		for c := run.Col; c < run.Col+run.Len && c < cols; c++ {
			g.Cells[run.Row][c].FG = run.Color
		}
	}
	return g
}

// DiffCells lists the cells that differ between two screens of one size, as the compared things differ (character, background,
// foreground), skipping the rows in skip.
func DiffCells(want, got *Grid, skip []int) [][2]int {
	var out [][2]int
	for r := 0; r < want.Rows && r < got.Rows; r++ {
		if containsInt(skip, r) {
			continue
		}
		for c := 0; c < want.Cols && c < got.Cols; c++ {
			if want.Cells[r][c] != got.Cells[r][c] {
				out = append(out, [2]int{r, c})
			}
		}
	}
	return out
}

func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

const rowH = int(cellH)

// SVG draws a screen as an image: a rectangle for each stretch of one background, a text for each stretch of one
// foreground. The cells in marks get a red frame (the cells that differ).
func (g *Grid) SVG(title string, marks [][2]int) string {
	w, h := float64(g.Cols)*cellW, float64(g.Rows)*cellH
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.1f %.1f" role="img" aria-label="%s" font-family="'DejaVu Sans Mono','Menlo','Consolas','Noto Sans Mono CJK JP','Noto Sans JP','Meiryo',monospace" font-size="16" style="white-space:pre">`+"\n", w, h, w, h, html.EscapeString(title))
	fmt.Fprintf(&b, `<rect width="%.1f" height="%.1f" fill="%s"/>`+"\n", w, h, g.Normal)
	for r := 0; r < g.Rows; r++ {
		for c := 0; c < g.Cols; {
			bg := g.Cells[r][c].BG
			e := c
			for e < g.Cols && g.Cells[r][e].BG == bg {
				e++
			}
			if bg != "" && bg != g.Normal {
				fmt.Fprintf(&b, `<rect x="%.1f" y="%d" width="%.1f" height="%d" fill="%s"/>`+"\n", float64(c)*cellW, r*rowH, float64(e-c)*cellW, rowH, bg)
			}
			c = e
		}
		for c := 0; c < g.Cols; {
			fg := g.Cells[r][c].FG
			e := c
			var text strings.Builder
			for e < g.Cols && g.Cells[r][e].FG == fg {
				text.WriteString(g.Cells[r][e].Ch)
				e++
			}
			if s := strings.TrimRight(text.String(), " "); s != "" && fg != "" {
				fmt.Fprintf(&b, `<text x="%.1f" y="%d" fill="%s" textLength="%.1f" lengthAdjust="spacing">%s</text>`+"\n", float64(c)*cellW, r*rowH+15, fg, float64(e-c)*cellW, html.EscapeString(strings.TrimLeft(s, " ")))
			}
			c = e
		}
	}
	for _, box := range marksByRow(marks) {
		fmt.Fprintf(&b, `<rect x="%.1f" y="%d" width="%.1f" height="%d" fill="none" stroke="#ff2d2d" stroke-width="2"/>`+"\n", float64(box[1])*cellW-1, box[0]*rowH, float64(box[2]-box[1]+1)*cellW+2, rowH)
	}
	b.WriteString("</svg>\n")
	return b.String()
}

// marksByRow merges the cells of one row into one box [row, first column, last column].
func marksByRow(marks [][2]int) [][3]int {
	byRow := map[int][2]int{}
	for _, m := range marks {
		if cur, ok := byRow[m[0]]; ok {
			byRow[m[0]] = [2]int{min(cur[0], m[1]), max(cur[1], m[1])}
		} else {
			byRow[m[0]] = [2]int{m[1], m[1]}
		}
	}
	var rows []int
	for r := range byRow {
		rows = append(rows, r)
	}
	sort.Ints(rows)
	var out [][3]int
	for _, r := range rows {
		out = append(out, [3]int{r, byRow[r][0], byRow[r][1]})
	}
	return out
}
