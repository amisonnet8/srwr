package uicheck

import (
	"encoding/json"
	"html"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const tinySVG = `<svg xmlns="http://www.w3.org/2000/svg" width="96" height="40" viewBox="0 0 48 20">
<rect width="96.0" height="40" fill="#222222"/>
<rect x="0.0" y="0" width="19.2" height="20" fill="#0b61a4"/>
<rect x="38.4" y="20" width="9.6" height="20" fill="#cfe3fb"/>
<rect x="9.6" y="-20" width="9.6" height="10" fill="#ff0000"/>
<text x="0.0" y="15" fill="#ffffff" font-weight="bold">a</text>
<text x="9.6" y="15" fill="#ffffff">&lt;</text>
<text x="19.2" y="15" fill="#d4d4d4">あ</text>
<text x="38.4" y="35" fill="#4aa3ff">●</text>
</svg>`

func TestParseSVG(t *testing.T) {
	g, err := ParseSVG(tinySVG)
	if err != nil {
		t.Fatal(err)
	}
	if g.Cols != 10 || g.Rows != 2 {
		t.Fatalf("size = %dx%d, want 10x2", g.Cols, g.Rows)
	}
	if got := g.RowText(0); got != "a<あ" {
		t.Errorf("row 0 = %q, want a<あ (the second cell of a wide character is skipped, and entities are read)", got)
	}
	if g.Cells[0][1].BG != "#0b61a4" || g.Cells[0][2].BG != "#222222" {
		t.Errorf("backgrounds = %s %s", g.Cells[0][1].BG, g.Cells[0][2].BG)
	}
	if g.Cells[1][4].Ch != "●" || g.Cells[1][4].BG != "#cfe3fb" || g.Cells[1][4].FG != "#4aa3ff" {
		t.Errorf("cell = %+v", g.Cells[1][4])
	}
	if _, err := ParseSVG("<svg></svg>"); err == nil {
		t.Error("an image without a background rectangle was accepted")
	}
}

func TestReadSVGTakesNormalFromTheCaller(t *testing.T) {
	// The image paints its most common background first. That is not the Normal background.
	g, err := ParseSVG(tinySVG)
	if err != nil {
		t.Fatal(err)
	}
	if g.Normal != "#222222" {
		t.Fatalf("Normal = %s", g.Normal)
	}
	g.Normal = "#1e1e1e"
	f := Reduce(g, "x")
	// every cell that is not #1e1e1e counts as painted
	var n int
	for _, r := range f.BG {
		n += r.Len
	}
	if n != g.Cols*g.Rows {
		t.Errorf("%d painted cells, want all %d (the main fill is not Normal here)", n, g.Cols*g.Rows)
	}
}

func TestParseCapture(t *testing.T) {
	cell := func(ch, fg, bg string, w int) []any { return []any{ch, fg, bg, false, w} }
	row := func(cells ...[]any) []any {
		out := make([]any, len(cells))
		for i, c := range cells {
			out[i] = c
		}
		return out
	}
	raw, _ := json.Marshal(map[string]any{"cols": 5, "rows": 2, "frames": []any{map[string]any{"label": "one", "rows": []any{
		row(cell("a", "#fff", "#000", 1), cell("あ", "#fff", "#111", 2), cell("b", "#fff", "#000", 1), cell(" ", "#fff", "#000", 1)),
		row(cell("●", "#4aa3ff", "#000", 1), cell("", "#fff", "#000", 1), cell(" ", "#fff", "#000", 1), cell(" ", "#fff", "#000", 1), cell(" ", "#fff", "#000", 1)),
	}}}})
	c, err := ParseCapture(raw, "#000")
	if err != nil {
		t.Fatal(err)
	}
	g := c.Grids[0]
	if g.RowText(0) != "aあb" || g.RowText(1) != "●" {
		t.Errorf("rows = %q %q", g.RowText(0), g.RowText(1))
	}
	if g.Cells[0][2].Ch != "" || g.Cells[0][2].BG != "#111" {
		t.Errorf("the second cell of a wide character = %+v", g.Cells[0][2])
	}
	if _, err := ParseCapture([]byte(`{"cols":1,"rows":2,"frames":[{"label":"x","rows":[[]]}]}`), "#000"); err == nil {
		t.Error("a frame with too few rows was accepted")
	}
}

func screen() *Grid {
	g := NewGrid(10, 5, "#1e1e1e")
	g.Put(0, 0, "ab", "#d4d4d4", "#1e1e1e")
	g.Put(1, 0, "why", "#ffffff", "#0b61a4")
	g.Put(1, 3, "   ", "", "#0b61a4")
	g.Put(2, 0, "●x", "#f0883e", "#1e1e1e")
	g.Put(3, 0, "status", "#1e1e1e", "#d4d4d4")
	g.Put(4, 0, "cmd", "#d4d4d4", "#1e1e1e")
	return g
}

func TestReduceChecksOnlyWhatSrwrColors(t *testing.T) {
	f := Reduce(screen(), "x")
	if f.Text[0] != "ab" || f.Text[1] != "why" || f.Text[3] != "status" {
		t.Errorf("text = %q", f.Text)
	}
	fg := cellsOf(f.FG)
	for _, want := range [][2]int{{1, 0}, {1, 2}, {2, 0}, {3, 0}, {3, 5}} {
		if _, ok := fg[want]; !ok {
			t.Errorf("foreground of %v is not checked", want)
		}
	}
	for _, not := range [][2]int{{0, 0}, {2, 1}, {1, 3}} {
		if _, ok := fg[not]; ok {
			t.Errorf("foreground of %v is checked: it is Vim's own syntax color, or a space", not)
		}
	}
	bg := cellsOf(f.BG)
	if bg[[2]int{1, 4}] != "#0b61a4" || bg[[2]int{0, 0}] != "" {
		t.Errorf("backgrounds = %v", bg)
	}
}

func TestCompare(t *testing.T) {
	want := Reduce(screen(), "x")
	if d := Compare(want, Reduce(screen(), "x"), "#1e1e1e", 10); len(d) != 0 {
		t.Errorf("the same screen differs: %v", d)
	}
	tests := []struct {
		name   string
		change func(g *Grid)
		want   string
	}{
		{"text", func(g *Grid) { g.Put(0, 0, "ax", "#d4d4d4", "#1e1e1e") }, "row 1 text"},
		{"background", func(g *Grid) { g.Cells[1][5].BG = "#b45f06" }, "background: 1 cells differ: (2,6) want \"#0b61a4\" got \"#b45f06\""},
		{"a missing paint", func(g *Grid) { g.Cells[1][5].BG = "#1e1e1e" }, "background: 1 cells differ"},
		{"foreground", func(g *Grid) { g.Cells[2][0].FG = "#4aa3ff" }, "foreground: 1 cells differ: (3,1)"},
		{"syntax color is not compared", func(g *Grid) { g.Cells[0][0].FG = "#ff0000" }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := screen()
			tt.change(g)
			d := strings.Join(Compare(want, Reduce(g, "x"), "#1e1e1e", 10), "\n")
			if tt.want == "" && d != "" || !strings.Contains(d, tt.want) {
				t.Errorf("differences = %q, want it to contain %q", d, tt.want)
			}
		})
	}
	t.Run("a skipped row", func(t *testing.T) {
		w := want
		w.Skip = []int{3}
		g := screen()
		g.Put(3, 0, "other!", "#ff0000", "#00ff00")
		if d := Compare(w, Reduce(g, "x"), "#1e1e1e", 10); len(d) != 0 {
			t.Errorf("a skipped row was compared: %v", d)
		}
	})
	t.Run("at most max lines", func(t *testing.T) {
		g := screen()
		for r := 0; r < 5; r++ {
			g.Put(r, 0, "zzzz", "#000000", "#123456")
		}
		if d := Compare(want, Reduce(g, "x"), "#1e1e1e", 2); len(d) != 2 {
			t.Errorf("%d lines, want 2", len(d))
		}
	})
}

func TestBaselineRoundTrip(t *testing.T) {
	b := &Baseline{Cols: 10, Rows: 5, Normal: "#1e1e1e", Frames: []Frame{Reduce(screen(), "a"), Reduce(screen(), "b")}}
	b.Frames[1].Skip = []int{3}
	data, err := MarshalBaseline(b)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "\n") != 4 {
		t.Errorf("one frame per line is expected:\n%s", data)
	}
	if strings.Contains(string(data), `\u00`) {
		t.Errorf("characters are escaped:\n%s", data)
	}
	got, err := ReadBaseline(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Frames) != 2 || got.Frames[1].Skip[0] != 3 || len(Compare(b.Frames[0], got.Frames[0], got.Normal, 5)) != 0 {
		t.Errorf("round trip = %+v", got)
	}
	if err := json.Unmarshal([]byte(`[1,2]`), new(Run)); err == nil {
		t.Error("a run with two items was accepted")
	}
}

func TestLiveStatus(t *testing.T) {
	g := NewGrid(100, 3, "#1e1e1e")
	for c := 0; c < 100; c++ {
		g.Cells[1][c] = Cell{Ch: " ", BG: "#d4d4d4"}
	}
	g.Put(1, 10, "old", "#1e1e1e", "#d4d4d4")
	if err := LiveStatus(g, 1, 10, 2, 3, 1, "text.go:103"); err != nil {
		t.Fatal(err)
	}
	if got := g.RowText(1); !strings.Contains(got, "srwr  2/3  [[ 戻る  ]] 進む   L：LIVE に戻る（新着 1）   text.go:103") || strings.Contains(got, "old") {
		t.Errorf("status = %q", got)
	}
	var orange int
	for _, c := range g.Cells[1] {
		if c.BG == "#b45f06" {
			orange++
		}
	}
	// " L：LIVE に戻る（新着 1） " is 26 cells (the wide characters take two).
	if orange != 26 {
		t.Errorf("orange cells = %d, want 26", orange)
	}
	if err := LiveStatus(g, 0, 0, 1, 1, 0, ""); err == nil {
		t.Error("a row that is not a status line was accepted")
	}
}

func TestPaintWindowRange(t *testing.T) {
	g := NewGrid(10, 5, "#000000")
	PaintWindowRange(g, 1, 3, 4, "#123456")
	if g.Cells[1][4].BG != "#123456" || g.Cells[3][9].BG != "#123456" || g.Cells[1][3].BG != "#000000" || g.Cells[0][5].BG != "#000000" || g.Cells[4][5].BG != "#000000" {
		t.Error("the wrong cells were painted")
	}
}

func TestWidth(t *testing.T) {
	if Width('a') != 1 || Width('あ') != 2 || Width('●') != 1 || Width('◆') != 1 || Width('（') != 2 {
		t.Error("width of a, あ, ●, ◆, （")
	}
}

func TestDotFirst(t *testing.T) {
	g := NewGrid(20, 4, "#000000")
	g.Put(1, 0, " 5 ", "#d4d4d4", "#000000")
	g.Put(1, 3, "●", "#f0883e", "#000000")
	g.Put(1, 4, " replace", "#d4d4d4", "#000000")
	g.Put(2, 0, "~", "#0000ff", "#000000")
	DotFirst(g, 1, 2)
	if got := g.RowText(1); got != "●  5 replace" {
		t.Errorf("row = %q", got)
	}
	if g.Cells[1][0].FG != "#f0883e" || g.Cells[1][2].FG != "#d4d4d4" {
		t.Error("the colors did not move with the cells")
	}
	if g.RowText(2) != "~" {
		t.Error("a row that is not of the list changed")
	}
}

// A short text in a long run of one color is drawn over its own cells: stretching it over the blanks around it spread its
// letters over the whole line (the Vim screens of R10.5 looked like noise).
func TestSVGTextIsStretchedOverItsOwnCellsOnly(t *testing.T) {
	g := NewGrid(30, 2, "#1e1e1e")
	g.Put(0, 0, "ab   textkit                ", "#d4d4d4", "#1e1e1e") // one color for the whole line, with blanks inside and around
	g.Put(1, 3, "あい  x", "#ffffff", "#1e1e1e")
	svg := g.SVG("t", nil)
	re := regexp.MustCompile(`x="([\d.]+)" y="\d+" fill="#\w+" textLength="([\d.]+)"[^>]*>([^<]*)</text>`)
	seen := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(svg, -1) {
		text := html.UnescapeString(m[3])
		cells := 0
		for _, r := range text {
			cells += Width(r)
		}
		if want := strconv.FormatFloat(float64(cells)*cellW, 'f', 1, 64); m[2] != want {
			t.Errorf("%q is drawn over %s, want %s (its own cells)", text, m[2], want)
		}
		seen[text] = m[1]
	}
	// The text starts at its first visible cell, and keeps the blanks inside it.
	if seen["ab   textkit"] != "0.0" || seen["あい  x"] != "28.8" {
		t.Errorf("texts = %v\n%s", seen, svg)
	}
}
