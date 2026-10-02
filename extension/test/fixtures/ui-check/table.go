package textkit

import "strings"

// Column は表の1列の定義。MaxWidth が 0 なら幅は無制限。
type Column struct {
	Header   string
	Align    Align
	MaxWidth int
}

// Table は見出しと行を持つ表。
type Table struct {
	Columns []Column
	Rows    [][]string
}

// AddRow は行を追加する。欄が足りなければ空で補い、多ければ切り捨てる。
func (t *Table) AddRow(cells ...string) {
	row := make([]string, len(t.Columns))
	copy(row, cells)
	t.Rows = append(t.Rows, row)
}

func (t *Table) widths() []int {
	ws := make([]int, len(t.Columns))
	for i, c := range t.Columns {
		ws[i] = DisplayWidth(c.Header)
		for _, row := range t.Rows {
			if w := DisplayWidth(row[i]); w > ws[i] {
				ws[i] = w
			}
		}
		if c.MaxWidth > 0 && ws[i] > c.MaxWidth {
			ws[i] = c.MaxWidth
		}
	}
	return ws
}

// Render は表を文字列にする。各行は改行で終わる。
func (t *Table) Render() string {
	ws := t.widths()
	var b strings.Builder
	line := func(cells []string) {
		parts := make([]string, len(cells))
		for i, c := range cells {
			parts[i] = Pad(Truncate(c, ws[i]), ws[i], t.Columns[i].Align)
		}
		b.WriteString("| " + strings.Join(parts, " | ") + " |\n")
	}
	headers := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		headers[i] = c.Header
	}
	line(headers)
	seps := make([]string, len(ws))
	for i, w := range ws {
		seps[i] = strings.Repeat("-", w)
	}
	b.WriteString("|-" + strings.Join(seps, "-|-") + "-|\n")
	for _, row := range t.Rows {
		line(row)
	}
	return b.String()
}
