package main

import (
	"fmt"
	"strings"

	"github.com/amisonnet8/srwr/internal/uicheck"
)

// whyRows are the why rows of a screen of the Vim client: the text of the cells with the select (blue) or replace (orange)
// background, row by row.
func whyRows(g *uicheck.Grid) []string {
	var rows []string
	for r := 1; r < g.Rows-2; r++ {
		var b strings.Builder
		for _, c := range g.Cells[r] {
			if c.BG == "#0b61a4" || c.BG == "#b45f06" {
				b.WriteString(c.Ch)
			}
		}
		if b.Len() > 0 {
			rows = append(rows, strings.TrimRight(b.String(), " "))
		}
	}
	return rows
}

// verifyLongWhy checks a screen that shows a long why: it is cut into rows (more than one), the first starts with the mark,
// the following are indented under it, and all of the why is there. It returns the number of why rows.
func verifyLongWhy(g *uicheck.Grid, why string) (int, []string) {
	rows := whyRows(g)
	var problems []string
	if len(rows) < 2 {
		problems = append(problems, fmt.Sprintf("the why is on %d rows, a long why must be folded onto more", len(rows)))
	}
	var joined strings.Builder
	for i, row := range rows {
		switch {
		case i == 0 && !strings.HasPrefix(row, "◆ "):
			problems = append(problems, fmt.Sprintf("the first why row does not start with the mark: %q", row))
		case i > 0 && !strings.HasPrefix(row, "  "):
			problems = append(problems, fmt.Sprintf("why row %d is not indented under the first: %q", i+1, row))
		}
		joined.WriteString(strings.TrimLeft(strings.TrimPrefix(row, "◆"), " "))
	}
	if strip(joined.String()) != strip(why) {
		problems = append(problems, fmt.Sprintf("the why on the screen is not all of the why:\n  screen %q\n  tape   %q", joined.String(), why))
	}
	return len(rows), problems
}

func strip(s string) string { return strings.Join(strings.Fields(s), "") }
