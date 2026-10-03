package uicheck

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func oneChar(ch string) *Grid {
	g := NewGrid(4, 2, "#000000")
	g.Put(0, 0, ch, "#ffffff", "")
	return g
}

func TestAnimatedSVGStepsThroughTheScreens(t *testing.T) {
	out, err := AnimatedSVG("demo", []*Grid{oneChar("a"), oneChar("b"), oneChar("c")}, []float64{1, 2, 1})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out, "<animate "); n != 3 {
		t.Errorf("%d animations, want 3", n)
	}
	if !strings.Contains(out, `dur="4.00s"`) {
		t.Errorf("the whole time is the sum of the times (4s):\n%s", out)
	}
	// Screen 2 is visible from 1/4 to 3/4 of the time.
	if !strings.Contains(out, `values="hidden;visible;hidden;hidden" keyTimes="0.0000;0.2500;0.7500;1.0000"`) {
		t.Errorf("the middle screen:\n%s", out)
	}
	// The first starts visible and the last stays visible to the end: both are in view when the loop turns.
	if !strings.Contains(out, `values="visible;hidden;hidden" keyTimes="0.0000;0.2500;1.0000"`) {
		t.Errorf("the first screen:\n%s", out)
	}
	if !strings.Contains(out, `values="hidden;visible;visible" keyTimes="0.0000;0.7500;1.0000"`) {
		t.Errorf("the last screen:\n%s", out)
	}
	// Exactly one screen is visible at any moment: the visible stretches tile 0..1.
	re := regexp.MustCompile(`values="([^"]+)" keyTimes="([^"]+)"`)
	seen := make([]bool, 100)
	for _, m := range re.FindAllStringSubmatch(out, -1) {
		vals, times := strings.Split(m[1], ";"), strings.Split(m[2], ";")
		for i := 0; i+1 < len(vals); i++ {
			if vals[i] != "visible" {
				continue
			}
			a, _ := strconv.ParseFloat(times[i], 64)
			b, _ := strconv.ParseFloat(times[i+1], 64)
			for k := int(a * 100); k < int(b*100); k++ {
				if seen[k] {
					t.Errorf("two screens are visible at %d%%", k)
				}
				seen[k] = true
			}
		}
	}
	for k := 0; k < 100; k++ {
		if !seen[k] {
			t.Errorf("no screen is visible at %d%%", k)
		}
	}
}

func TestAnimatedSVGRefusesWhatCannotBeDrawn(t *testing.T) {
	cases := []struct {
		name  string
		grids []*Grid
		holds []float64
	}{
		{"none", nil, nil},
		{"times do not match", []*Grid{oneChar("a")}, []float64{1, 1}},
		{"zero time", []*Grid{oneChar("a")}, []float64{0}},
		{"sizes differ", []*Grid{oneChar("a"), NewGrid(5, 2, "#000000")}, []float64{1, 1}},
	}
	for _, c := range cases {
		if _, err := AnimatedSVG("x", c.grids, c.holds); err == nil {
			t.Errorf("%s: no error", c.name)
		}
	}
}

func TestSVGIsUnchangedByTheSplit(t *testing.T) {
	g := oneChar("a")
	out := g.SVG("t", nil)
	if !strings.HasPrefix(out, "<svg ") || !strings.HasSuffix(out, "</svg>\n") || strings.Count(out, "<svg ") != 1 {
		t.Errorf("SVG:\n%s", out)
	}
}

func TestWithoutTopRows(t *testing.T) {
	g := NewGrid(3, 3, "#000000")
	g.Put(0, 0, "a", "#fff", "")
	g.Put(1, 0, "b", "#fff", "")
	h := g.WithoutTopRows(1)
	if h.Rows != 2 || h.Cells[0][0].Ch != "b" || g.Rows != 3 {
		t.Errorf("rows %d, first %q; the original has %d rows", h.Rows, h.Cells[0][0].Ch, g.Rows)
	}
	if g.WithoutTopRows(9).Rows != 3 {
		t.Error("too many rows to remove should keep the screen")
	}
}
