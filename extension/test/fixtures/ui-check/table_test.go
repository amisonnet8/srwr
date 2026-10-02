package textkit

import (
	"reflect"
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	tb := &Table{Columns: []Column{
		{Header: "Name"},
		{Header: "Qty", Align: Right},
		{Header: "Note", MaxWidth: 5},
	}}
	tb.AddRow("apple", "3", "ok")
	tb.AddRow("kiwi", "12", "exactly")
	tb.AddRow("fig", "7", "12345")
	want := "" +
		"| Name  | Qty | Note  |\n" +
		"|-------|-----|-------|\n" +
		"| apple |   3 | ok    |\n" +
		"| kiwi  |  12 | exac… |\n" +
		"| fig   |   7 | 12345 |\n"
	if got := tb.Render(); got != want {
		t.Errorf("Render =\n%s\nwant\n%s", got, want)
	}

	tb = &Table{Columns: []Column{
		{Header: "品名"},
		{Header: "価格", Align: Right},
		{Header: "備考", Align: Center, MaxWidth: 6},
	}}
	tb.AddRow("りんご", "120", "国産")
	tb.AddRow("バナナ", "80", "フィリピン産")
	tb.AddRow("みかん", "60", "とても甘い！")
	want = "" +
		"| 品名   | 価格 |  備考  |\n" +
		"|--------|------|--------|\n" +
		"| りんご |  120 |  国産  |\n" +
		"| バナナ |   80 | フィ…  |\n" +
		"| みかん |   60 | とて…  |\n"
	if got := tb.Render(); got != want {
		t.Errorf("Render =\n%s\nwant\n%s", got, want)
	}
}

func TestAddRowShapes(t *testing.T) {
	tb := &Table{Columns: []Column{{Header: "a"}, {Header: "b"}}}
	tb.AddRow("1")
	tb.AddRow("1", "2", "3")
	for i, row := range tb.Rows {
		if len(row) != 2 {
			t.Errorf("行 %d の欄数 = %d, want 2", i, len(row))
		}
	}
	if tb.Rows[0][1] != "" || tb.Rows[1][1] != "2" {
		t.Errorf("rows = %q", tb.Rows)
	}
}

func TestParseRecord(t *testing.T) {
	cases := []struct {
		name string
		line string
		sep  rune
		want []string
	}{
		{"単純", "a,b,c", ',', []string{"a", "b", "c"}},
		{"途中の空欄", "a,,c", ',', []string{"a", "", "c"}},
		{"末尾の空欄", "a,b,", ',', []string{"a", "b", ""}},
		{"空欄だけ", ",", ',', []string{"", ""}},
		{"空文字列", "", ',', []string{""}},
		{"引用符", `"x,y",z`, ',', []string{"x,y", "z"}},
		{"引用符内の引用符", `"say ""hi""",z`, ',', []string{`say "hi"`, "z"}},
		{"末尾の空の引用欄", `a,""`, ',', []string{"a", ""}},
		{"タブ区切り", "a\tb c\td", '\t', []string{"a", "b c", "d"}},
		{"日本語", "名前;価値;", ';', []string{"名前", "価値", ""}},
	}
	for _, c := range cases {
		got, err := ParseRecord(c.line, c.sep)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: ParseRecord(%q) = %q, want %q", c.name, c.line, got, c.want)
		}
	}
}

func TestParseRecordUnterminated(t *testing.T) {
	if _, err := ParseRecord(`a,"bc`, ','); err == nil {
		t.Error("引用符が閉じていなければエラーになるべき")
	}
}

func TestParseTable(t *testing.T) {
	src := "name,qty,note\r\napple,3,\r\n\r\nkiwi,12,\"a,b\"\r\n"
	tb, err := ParseTable(src, ',')
	if err != nil {
		t.Fatal(err)
	}
	if len(tb.Columns) != 3 || tb.Columns[2].Header != "note" {
		t.Errorf("columns = %+v", tb.Columns)
	}
	want := [][]string{{"apple", "3", ""}, {"kiwi", "12", "a,b"}}
	if !reflect.DeepEqual(tb.Rows, want) {
		t.Errorf("rows = %q, want %q", tb.Rows, want)
	}

	tb, err = ParseTable("id,name,\n1,a,x\n", ',')
	if err != nil {
		t.Fatal(err)
	}
	if len(tb.Columns) != 3 {
		t.Fatalf("列数 = %d, want 3", len(tb.Columns))
	}
	if got := tb.Rows[0]; !reflect.DeepEqual(got, []string{"1", "a", "x"}) {
		t.Errorf("row = %q", got)
	}
}

func TestParseTableErrors(t *testing.T) {
	if _, err := ParseTable("\n \n", ','); err == nil {
		t.Error("見出しがなければエラーになるべき")
	}
	_, err := ParseTable("a,b\n1,\"2\n", ',')
	if err == nil || !strings.Contains(err.Error(), "2行目") {
		t.Errorf("err = %v, want 2行目 を含むエラー", err)
	}
}
