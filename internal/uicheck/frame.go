package uicheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Run is a stretch of cells on one row with the same color: [row, column, length, color] in the baseline file.
type Run struct {
	Row, Col, Len int
	Color         string
}

// MarshalJSON writes a Run as [row, column, length, color], which keeps the baseline small.
func (r Run) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{r.Row, r.Col, r.Len, r.Color})
}

// UnmarshalJSON reads what MarshalJSON wrote.
func (r *Run) UnmarshalJSON(b []byte) error {
	var a []any
	if err := json.Unmarshal(b, &a); err != nil || len(a) != 4 {
		return fmt.Errorf("a run is [row, column, length, color]: %s", b)
	}
	row, ok1 := a[0].(float64)
	col, ok2 := a[1].(float64)
	n, ok3 := a[2].(float64)
	color, ok4 := a[3].(string)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return fmt.Errorf("a run is [row, column, length, color]: %s", b)
	}
	*r = Run{int(row), int(col), int(n), color}
	return nil
}

// Frame is a screen reduced to what the baseline checks:
//   - Text: every row's text
//   - BG: the background of every cell that is not the Normal background
//   - FG: the foreground of the cells whose color srwr decides (see Checked); the others are Vim's syntax colors,
//     which follow the Vim version
type Frame struct {
	Label string   `json:"label"`
	Skip  []int    `json:"skip,omitempty"` // rows not compared, each with the reason in vim/test/baseline/README.md
	Text  []string `json:"text"`
	BG    []Run    `json:"bg"`
	FG    []Run    `json:"fg"`
}

// Baseline is the file vim/test/baseline/*.json.
type Baseline struct {
	Cols   int     `json:"cols"`
	Rows   int     `json:"rows"`
	Normal string  `json:"normal"`
	Frames []Frame `json:"frames"`
}

// srwrBackgrounds are the backgrounds srwr gives: the why rows, the ranges and the current row of the list.
var srwrBackgrounds = map[string]bool{
	"#0b61a4": true, "#b45f06": true, "#1d3a5c": true, "#583c27": true, "#cfe3fb": true, "#fde3c8": true, "#3a3d41": true, "#e4e6f1": true,
}

// Checked tells whether the foreground of a cell is compared: a dot of the list, a cell srwr painted, and the status
// line. The text colored by Vim's syntax files is not, because it changes with the version of Vim.
func Checked(g *Grid, row, col int) bool {
	c := g.Cells[row][col]
	if c.Ch == "" || c.Ch == " " {
		return false
	}
	return c.Ch == "●" || srwrBackgrounds[c.BG] || row == g.Rows-2
}

// Reduce makes the Frame of a screen.
func Reduce(g *Grid, label string) Frame {
	f := Frame{Label: label}
	for r := 0; r < g.Rows; r++ {
		f.Text = append(f.Text, g.RowText(r))
		f.BG = append(f.BG, runs(g, r, func(c int) (string, bool) {
			bg := g.Cells[r][c].BG
			return bg, bg != "" && bg != g.Normal
		})...)
		f.FG = append(f.FG, runs(g, r, func(c int) (string, bool) {
			return g.Cells[r][c].FG, Checked(g, r, c)
		})...)
	}
	return f
}

func runs(g *Grid, row int, color func(col int) (string, bool)) []Run {
	var out []Run
	for c := 0; c < g.Cols; c++ {
		v, ok := color(c)
		if !ok {
			continue
		}
		if n := len(out); n > 0 && out[n-1].Row == row && out[n-1].Col+out[n-1].Len == c && out[n-1].Color == v {
			out[n-1].Len++
			continue
		}
		out = append(out, Run{row, c, 1, v})
	}
	return out
}

func cellsOf(rs []Run) map[[2]int]string {
	m := map[[2]int]string{}
	for _, r := range rs {
		for c := r.Col; c < r.Col+r.Len; c++ {
			m[[2]int{r.Row, c}] = r.Color
		}
	}
	return m
}

// Compare lists how got differs from want, at most max lines; nothing means they are the same.
func Compare(want, got Frame, normal string, max int) []string {
	skip := func(row int) bool { return slices.Contains(want.Skip, row) }
	var out []string
	add := func(format string, a ...any) {
		if len(out) < max {
			out = append(out, fmt.Sprintf(format, a...))
		}
	}
	for r := range want.Text {
		if r >= len(got.Text) {
			add("row %d: missing", r+1)
			continue
		}
		if !skip(r) && want.Text[r] != got.Text[r] {
			add("row %d text:\n    want %q\n    got  %q", r+1, want.Text[r], got.Text[r])
		}
	}
	for _, c := range []struct {
		name       string
		want, got  []Run
		defaultVal string
	}{{"background", want.BG, got.BG, normal}, {"foreground", want.FG, got.FG, ""}} {
		w, g := cellsOf(c.want), cellsOf(c.got)
		for _, m := range []map[[2]int]string{w, g} {
			for k := range m {
				if skip(k[0]) {
					delete(m, k)
				}
			}
		}
		var keys [][2]int
		for k := range w {
			keys = append(keys, k)
		}
		for k := range g {
			if _, ok := w[k]; !ok {
				keys = append(keys, k)
			}
		}
		sort.Slice(keys, func(i, j int) bool { return slices.Compare(keys[i][:], keys[j][:]) < 0 })
		var bad []string
		for _, k := range keys {
			wv, wok := w[k]
			gv, gok := g[k]
			if !wok {
				wv = c.defaultVal
			}
			if !gok {
				gv = c.defaultVal
			}
			if wv != gv {
				bad = append(bad, fmt.Sprintf("(%d,%d) want %q got %q", k[0]+1, k[1]+1, wv, gv))
			}
		}
		if len(bad) > 0 {
			shown := bad
			if len(shown) > 4 {
				shown = shown[:4]
			}
			add("%s: %d cells differ: %s", c.name, len(bad), strings.Join(shown, "; "))
		}
	}
	return out
}

// ReadBaseline reads a baseline file.
func ReadBaseline(data []byte) (*Baseline, error) {
	var b Baseline
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

// MarshalBaseline writes a baseline file: one frame per line, so a change shows in a diff as the frames that changed.
func MarshalBaseline(b *Baseline) ([]byte, error) {
	var out strings.Builder
	head, err := json.Marshal(struct {
		Cols   int    `json:"cols"`
		Rows   int    `json:"rows"`
		Normal string `json:"normal"`
	}{b.Cols, b.Rows, b.Normal})
	if err != nil {
		return nil, err
	}
	out.WriteString(strings.TrimSuffix(string(head), "}") + `,"frames":[` + "\n")
	for i, f := range b.Frames {
		var line bytes.Buffer
		enc := json.NewEncoder(&line)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(f); err != nil {
			return nil, err
		}
		out.WriteString(strings.TrimSuffix(line.String(), "\n"))
		if i < len(b.Frames)-1 {
			out.WriteString(",")
		}
		out.WriteString("\n")
	}
	out.WriteString("]}\n")
	return []byte(out.String()), nil
}
