package uicheck

import (
	"encoding/json"
	"strings"
	"testing"
)

func shotOf(t *testing.T, js string) Shot {
	t.Helper()
	var s Shot
	if err := json.Unmarshal([]byte(js), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

const shotTab = `{"label":"5","frame":4,"status":[{"text":"5/7"}],"viewDescription":"x","tree":[],"quickPick":null,"tabs":[{"uri":"srwr-replay:/t/a.go","column":1,"text":"one\ntwo\nthree","options":{"lineNumbers":0},"reveal":2,"decorations":[{"opts":{"isWholeLine":true,"backgroundColor":"COLOR"},"ranges":[{"line":1}]}]}]}`

func TestCompareShotsNamesWhatDiffers(t *testing.T) {
	want := []Shot{shotOf(t, strings.ReplaceAll(shotTab, "COLOR", "#b45f06"))}
	if d := CompareShots(want, want); len(d) != 0 {
		t.Fatalf("the same shots differ: %v", d)
	}
	got := []Shot{shotOf(t, strings.ReplaceAll(shotTab, "COLOR", "#0b61a4"))}
	d := CompareShots(want, got)
	if len(d) != 1 || d[0].Index != 1 || d[0].Label != "5" {
		t.Fatalf("diffs = %+v", d)
	}
	msg := strings.Join(d[0].Diffs, "\n")
	for _, w := range []string{"decorations", "#b45f06", "#0b61a4", "line 1", "srwr-replay:/t/a.go"} {
		if !strings.Contains(msg, w) {
			t.Errorf("the message lacks %q: %s", w, msg)
		}
	}
	// the document, the scroll position and the bar are compared too
	changed := strings.NewReplacer("three", "THREE", `"reveal":2`, `"reveal":9`, "5/7", "6/7").Replace(strings.ReplaceAll(shotTab, "COLOR", "#b45f06"))
	msg = strings.Join(CompareShots(want, []Shot{shotOf(t, changed)})[0].Diffs, "\n")
	for _, w := range []string{"document: line 3", "reveal", "status"} {
		if !strings.Contains(msg, w) {
			t.Errorf("the message lacks %q: %s", w, msg)
		}
	}
	if d := CompareShots(want, nil); len(d) != 1 || d[0].Index != 0 {
		t.Errorf("a different number of frames: %+v", d)
	}
}

func TestShotSVGDrawsThePaintedLines(t *testing.T) {
	svg := ShotSVG(shotOf(t, strings.ReplaceAll(shotTab, "COLOR", "#b45f06")), "t")
	if !strings.Contains(svg, `fill="#b45f06"`) || !strings.Contains(svg, "two") || !strings.Contains(svg, "5/7") {
		t.Errorf("the picture lacks the color, the text or the bar:\n%s", svg)
	}
}

func TestGridOfAndDiffCells(t *testing.T) {
	f := Frame{Text: []string{"ab", "あい"}, BG: []Run{{1, 0, 2, "#0b61a4"}}, FG: []Run{{0, 0, 1, "#ff0000"}}}
	g := GridOf(f, 6, 2, "#1e1e1e", "#d4d4d4")
	if g.Cells[0][0].FG != "#ff0000" || g.Cells[0][1].FG != "#d4d4d4" || g.Cells[1][0].BG != "#0b61a4" || g.Cells[1][4].BG != "#1e1e1e" {
		t.Errorf("cells = %+v", g.Cells)
	}
	h := GridOf(f, 6, 2, "#1e1e1e", "#d4d4d4")
	if d := DiffCells(g, h, nil); len(d) != 0 {
		t.Errorf("equal screens differ at %v", d)
	}
	h.Cells[0][1].Ch = "x"
	h.Cells[1][0].BG = "#b45f06"
	if d := DiffCells(g, h, nil); len(d) != 2 {
		t.Errorf("diff cells = %v", d)
	}
	if d := DiffCells(g, h, []int{1}); len(d) != 1 {
		t.Errorf("a skipped row still counted: %v", d)
	}
	svg := g.SVG("t", [][2]int{{0, 1}, {0, 3}})
	if strings.Count(svg, `stroke="#ff2d2d"`) != 1 || !strings.Contains(svg, "#0b61a4") {
		t.Errorf("one box per row with the marks, and the background drawn:\n%s", svg)
	}
}

func TestCompareBaselineNamesTheScreen(t *testing.T) {
	frame := func(row0, row1 string) Frame {
		return Frame{Label: "1", Text: []string{row0, row1}, BG: []Run{{1, 0, 3, "#0b61a4"}}, FG: []Run{{1, 0, 3, "#d4d4d4"}}}
	}
	want := &Baseline{Cols: 6, Rows: 2, Normal: "#1e1e1e", Frames: []Frame{frame("[無名]", "abc"), frame("[無名]", "abc")}}
	grid := func(row0, row1, bg string) *Grid {
		g := NewGrid(6, 2, "#1e1e1e")
		g.Put(0, 0, row0, "#d4d4d4", "#1e1e1e")
		g.Put(1, 0, row1, "#d4d4d4", bg)
		return g
	}
	same := &Capture{Labels: []string{"1", "2"}, Grids: []*Grid{grid("[No Name]"[:6], "abc", "#0b61a4"), grid("x", "abc", "#0b61a4")}}
	if d := CompareBaseline(want, same); len(d) != 0 {
		t.Errorf("the tab line (row 1) is not compared, yet: %+v", d)
	}
	other := &Capture{Labels: []string{"1", "2"}, Grids: []*Grid{grid("x", "abc", "#0b61a4"), grid("x", "abc", "#b45f06")}}
	d := CompareBaseline(want, other)
	if len(d) != 1 || d[0].Index != 2 || !strings.Contains(strings.Join(d[0].Diffs, "\n"), "background") {
		t.Errorf("diffs = %+v", d)
	}
	if d := CompareBaseline(want, &Capture{Labels: []string{"1"}, Grids: []*Grid{grid("x", "abc", "#0b61a4")}}); len(d) != 1 || d[0].Index != 0 || !strings.Contains(d[0].Diffs[0], "1 screens") {
		t.Errorf("a different number of screens: %+v", d)
	}
}
