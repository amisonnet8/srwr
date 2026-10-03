package uicheck

import (
	"fmt"
	"html"
	"math"
	"os"
	"regexp"
	"strconv"
)

// The approved images were drawn from real screens: a rectangle for each run of cells with
// a background and a <text> for each character, 9.6 by 20 units a cell.
const (
	cellW = 9.6
	cellH = 20.0
)

var (
	svgSize = regexp.MustCompile(`<rect width="([\d.]+)" height="([\d.]+)" fill="(#\w+)"`)
	svgRect = regexp.MustCompile(`<rect x="([\d.]+)" y="([\d.]+)" width="([\d.]+)" height="([\d.]+)" fill="(#\w+)"`)
	svgText = regexp.MustCompile(`<text x="([\d.]+)" y="([\d.]+)" fill="(#\w+)"[^>]*>(.*?)</text>`)
)

// ReadSVG reads an approved image as a screen. normal is the background of the Normal group the image was drawn with.
func ReadSVG(path, normal string) (*Grid, error) {
	b, err := os.ReadFile(path) //nolint:gosec // a path the caller names
	if err != nil {
		return nil, err
	}
	g, err := ParseSVG(string(b))
	if err != nil {
		return nil, err
	}
	g.Normal = normal
	return g, nil
}

// ParseSVG reads the text of an image as a screen. The image paints the most common background first, whatever it is,
// so the Normal background is not known from the image: the caller gives it (ReadSVG).
func ParseSVG(s string) (*Grid, error) {
	m := svgSize.FindStringSubmatch(s)
	if m == nil {
		return nil, fmt.Errorf("no background rectangle")
	}
	w, _ := strconv.ParseFloat(m[1], 64)
	h, _ := strconv.ParseFloat(m[2], 64)
	g := NewGrid(int(math.Round(w/cellW)), int(math.Round(h/cellH)), m[3])
	for _, m := range svgRect.FindAllStringSubmatch(s, -1) {
		x, _ := strconv.ParseFloat(m[1], 64)
		y, _ := strconv.ParseFloat(m[2], 64)
		rw, _ := strconv.ParseFloat(m[3], 64)
		rh, _ := strconv.ParseFloat(m[4], 64)
		row := int(math.Round(y / cellH))
		if rh != cellH || row < 0 || row >= g.Rows {
			continue // the cursor, which is not part of the screen
		}
		for c := int(math.Round(x / cellW)); c < int(math.Round((x+rw)/cellW)) && c < g.Cols; c++ {
			g.Cells[row][c].BG = m[5]
		}
	}
	for _, m := range svgText.FindAllStringSubmatch(s, -1) {
		x, _ := strconv.ParseFloat(m[1], 64)
		y, _ := strconv.ParseFloat(m[2], 64)
		row, col := int(math.Round((y-15)/cellH)), int(math.Round(x/cellW))
		if row < 0 || row >= g.Rows || col < 0 || col >= g.Cols {
			continue
		}
		ch := html.UnescapeString(m[4])
		g.Cells[row][col].Ch, g.Cells[row][col].FG = ch, m[3]
		if r := []rune(ch); len(r) == 1 && Width(r[0]) == 2 && col+1 < g.Cols {
			g.Cells[row][col+1].Ch = ""
		}
	}
	return g, nil
}
