package textkit

import (
	"fmt"
	"strings"
)

// ParseRecord は区切り文字 sep で区切られた1行を欄に分ける。
// 欄は "..." で囲めて、中では sep や "" （二重引用符1つ）を書ける。
// 空の欄も1つの欄として数える（"a,,b" は3欄、"a," は2欄）。
func ParseRecord(line string, sep rune) ([]string, error) {
	var fields []string
	var cur strings.Builder
	inQuote := false
	rs := []rune(line)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case inQuote && r == '"':
			if i+1 < len(rs) && rs[i+1] == '"' {
				cur.WriteRune('"')
				i++
			} else {
				inQuote = false
			}
		case inQuote:
			cur.WriteRune(r)
		case r == '"':
			inQuote = true
		case r == sep:
			fields = append(fields, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if inQuote {
		return nil, fmt.Errorf("引用符が閉じていません: %q", line)
	}
	fields = append(fields, cur.String())
	return fields, nil
}

// ParseTable は先頭行を見出しとする表を読み込む。
// 空行は読み飛ばし、行末の "\r" は取り除く。
func ParseTable(src string, sep rune) (*Table, error) {
	var t *Table
	for n, line := range strings.Split(src, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields, err := ParseRecord(line, sep)
		if err != nil {
			return nil, fmt.Errorf("%d行目: %w", n+1, err)
		}
		if t == nil {
			cols := make([]Column, len(fields))
			for i, f := range fields {
				cols[i] = Column{Header: f}
			}
			t = &Table{Columns: cols}
			continue
		}
		t.AddRow(fields...)
	}
	if t == nil {
		return nil, fmt.Errorf("見出し行がありません")
	}
	return t, nil
}
