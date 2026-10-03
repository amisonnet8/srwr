package uicheck

import "fmt"

// LiveStatus redraws the status line of the right-hand window of a live view that went back to an older frame, the way
// the UI gate of R5 (dev/review/R5) decided: the close hint is left out when it does not fit, so the orange mark
// "L：LIVE に戻る（新着 N）" is whole. The approved image of this state (stage L4 to L6 of the live view) shows the old,
// cut-off line; this is the line that replaces it. col is where the window starts.
func LiveStatus(g *Grid, row, col, index, total, behind int, where string) error {
	base := g.Cells[row][col]
	if base.BG == "" || base.BG == g.Normal {
		return fmt.Errorf("row %d column %d is not on a status line", row+1, col+1)
	}
	for c := col; c < g.Cols; c++ {
		g.Cells[row][c] = Cell{Ch: " ", BG: base.BG}
	}
	c := col
	c = g.Put(row, c, fmt.Sprintf("srwr  %d/%d  ", index, total), base.FG, base.BG)
	c = g.Put(row, c, "[[ 戻る", base.FG, base.BG)
	c = g.Put(row, c, "  ", base.FG, base.BG)
	c = g.Put(row, c, "]] 進む", base.FG, base.BG)
	c = g.Put(row, c, "  ", base.FG, base.BG)
	c = g.Put(row, c, fmt.Sprintf(" L：LIVE に戻る（新着 %d） ", behind), "#ffffff", "#b45f06")
	g.Put(row, c, "  "+where, base.FG, base.BG)
	return nil
}

// PaintWindowRange paints the rows firstRow..lastRow from column col to the right edge with bg. The approved images of the
// frames whose range is longer than the window (period.go:1-64 and so on, in the tape without why) show no paint at all: the
// image was taken before the paint of so many lines was drawn. The range is to be painted (docs/reference/vim.md 3), so the
// baseline of those frames is made with it.
func PaintWindowRange(g *Grid, firstRow, lastRow, col int, bg string) {
	for r := firstRow; r <= lastRow; r++ {
		for c := col; c < g.Cols; c++ {
			g.Cells[r][c].BG = bg
		}
	}
}

// DotFirst puts the dot of every row of the operation list before the number. The approved images show the rows as
// " 5 ● replace ..." ; the person who reviews the UI decided the Vim list follows the VSCode list: "● 5 replace ...".
// The first five cells of a row hold the same things in the other order, so the cells are moved and nothing else changes.
// A row is one of the list if its fourth cell is the dot; the list is the left 40 columns, rows firstRow..lastRow.
func DotFirst(g *Grid, firstRow, lastRow int) {
	for r := firstRow; r <= lastRow; r++ {
		row := g.Cells[r]
		if row[3].Ch != "●" {
			continue
		}
		row[0], row[1], row[2], row[3] = row[3], row[2], row[0], row[1]
	}
}
