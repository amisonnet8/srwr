package uicheck

import (
	"fmt"
	"html"
	"strings"
)

// AnimatedSVG draws screens one after another as a single image that steps through them and starts again (SMIL, which a
// browser, and GitHub in an <img>, plays). holds[i] is how many seconds screen i stays. All the screens are the size of
// the first one.
func AnimatedSVG(title string, grids []*Grid, holds []float64) (string, error) {
	if len(grids) == 0 || len(grids) != len(holds) {
		return "", fmt.Errorf("%d screens and %d times", len(grids), len(holds))
	}
	total := 0.0
	for _, h := range holds {
		if h <= 0 {
			return "", fmt.Errorf("a screen must stay for a positive time, got %v", h)
		}
		total += h
	}
	first := grids[0]
	for i, g := range grids {
		if g.Cols != first.Cols || g.Rows != first.Rows {
			return "", fmt.Errorf("screen %d is %dx%d, the first is %dx%d", i, g.Cols, g.Rows, first.Cols, first.Rows)
		}
	}
	w, h := float64(first.Cols)*cellW, float64(first.Rows)*cellH
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.1f %.1f" role="img" aria-label="%s" font-family="%s" font-size="16" style="white-space:pre">`+"\n", w, h, w, h, html.EscapeString(title), svgFont)
	start := 0.0
	for i, g := range grids {
		end := start + holds[i]
		values, times := visibility(start/total, end/total)
		fmt.Fprintf(&b, `<g visibility="hidden"><animate attributeName="visibility" calcMode="discrete" values="%s" keyTimes="%s" dur="%.2fs" repeatCount="indefinite"/>`+"\n", values, times, total)
		b.WriteString(g.svgBody(nil))
		b.WriteString("</g>\n")
		start = end
	}
	b.WriteString("</svg>\n")
	return b.String(), nil
}

// visibility is the values and keyTimes of one screen that is visible from `from` to `to` (fractions of the whole time).
// The key times begin at 0 and end at 1, as SMIL asks.
func visibility(from, to float64) (values, times string) {
	var v, t []string
	add := func(value string, at float64) {
		v = append(v, value)
		t = append(t, fmt.Sprintf("%.4f", at))
	}
	if from > 0 {
		add("hidden", 0)
		add("visible", from)
	} else {
		add("visible", 0)
	}
	if to < 1 {
		add("hidden", to)
		add("hidden", 1)
	} else {
		add("visible", 1)
	}
	return strings.Join(v, ";"), strings.Join(t, ";")
}
