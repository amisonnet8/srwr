// Package uicheck turns screens into something that can be compared: a screen taken from a real Vim (a terminal
// grid) and a screen drawn in an approved SVG image become the same Frame, and two Frames can be compared cell by cell.
// It is used by the screen tests (vim/screen_test.go) and by tools/ui-check, which makes the baseline from the images.
package uicheck

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Cell is one cell of a screen. The second cell of a wide character has an empty Ch.
type Cell struct {
	Ch     string
	FG, BG string
}

// Grid is a screen: Rows rows of Cols cells. Normal is the background of a cell nothing was drawn on.
type Grid struct {
	Cols, Rows int
	Normal     string
	Cells      [][]Cell
}

// NewGrid makes a blank screen.
func NewGrid(cols, rows int, normal string) *Grid {
	g := &Grid{Cols: cols, Rows: rows, Normal: normal, Cells: make([][]Cell, rows)}
	for r := range g.Cells {
		g.Cells[r] = make([]Cell, cols)
		for c := range g.Cells[r] {
			g.Cells[r][c] = Cell{Ch: " ", BG: normal}
		}
	}
	return g
}

// Put writes text at (row, col) with the colors, the way a terminal would: a wide character takes two cells.
// It returns the column after the text.
func (g *Grid) Put(row, col int, text, fg, bg string) int {
	for _, r := range text {
		if col >= g.Cols {
			return col
		}
		g.Cells[row][col] = Cell{Ch: string(r), FG: fg, BG: bg}
		col++
		if Width(r) == 2 && col < g.Cols {
			g.Cells[row][col] = Cell{Ch: "", FG: fg, BG: bg}
			col++
		}
	}
	return col
}

// Width is the number of cells a character takes in a terminal: 2 for the wide ones.
func Width(r rune) int {
	switch {
	case r >= 0x1100 && r <= 0x115f, r >= 0x2e80 && r <= 0xa4cf, r >= 0xac00 && r <= 0xd7a3,
		r >= 0xf900 && r <= 0xfaff, r >= 0xfe30 && r <= 0xfe6f, r >= 0xff00 && r <= 0xff60, r >= 0xffe0 && r <= 0xffe6:
		return 2
	}
	return 1
}

// RowText is the text of a row: the second cell of a wide character is skipped and trailing spaces are cut.
func (g *Grid) RowText(row int) string {
	var b strings.Builder
	for _, c := range g.Cells[row] {
		b.WriteString(c.Ch)
	}
	return strings.TrimRight(b.String(), " ")
}

// captureFile is what vim/test/screen/capture.vim writes.
type captureFile struct {
	Cols   int `json:"cols"`
	Rows   int `json:"rows"`
	Frames []struct {
		Label string  `json:"label"`
		Rows  [][]any `json:"rows"`
	} `json:"frames"`
}

// Capture is a set of screens taken from a real Vim.
type Capture struct {
	Labels []string
	Grids  []*Grid
}

// ParseCapture reads the file capture.vim wrote. normal is the background of the Normal group of the inner Vim.
func ParseCapture(data []byte, normal string) (*Capture, error) {
	var f captureFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	out := &Capture{}
	for _, fr := range f.Frames {
		g := NewGrid(f.Cols, f.Rows, normal)
		if len(fr.Rows) != f.Rows {
			return nil, fmt.Errorf("frame %s: %d rows, want %d", fr.Label, len(fr.Rows), f.Rows)
		}
		for r, row := range fr.Rows {
			col := 0
			for _, raw := range row {
				cell, ok := raw.([]any)
				if !ok || len(cell) != 5 {
					return nil, fmt.Errorf("frame %s row %d: a cell is %v", fr.Label, r, raw)
				}
				ch, _ := cell[0].(string)
				fg, _ := cell[1].(string)
				bg, _ := cell[2].(string)
				w, _ := cell[4].(float64)
				if ch == "" {
					ch = " "
				}
				if utf8.RuneCountInString(ch) > 1 {
					ch = string([]rune(ch)[0])
				}
				col = g.Put(r, col, ch, fg, bg)
				if w == 2 && Width([]rune(ch)[0]) != 2 && col < g.Cols {
					// A character this table calls narrow but the terminal drew wide.
					g.Cells[r][col] = Cell{Ch: "", FG: fg, BG: bg}
					col++
				}
			}
		}
		out.Labels = append(out.Labels, fr.Label)
		out.Grids = append(out.Grids, g)
	}
	return out, nil
}

// WithoutTopRows returns the screen without its first n rows (for example the tab line of Vim, which says "[No Name]" in a
// terminal that was started for a picture).
func (g *Grid) WithoutTopRows(n int) *Grid {
	if n < 0 || n > g.Rows {
		n = 0
	}
	return &Grid{Cols: g.Cols, Rows: g.Rows - n, Normal: g.Normal, Cells: g.Cells[n:]}
}
