package hook

import (
	"reflect"
	"testing"

	"github.com/amisonnet8/srwr/internal/core"
)

func lines(a, b int) core.HookRange { return core.HookRange{Mode: core.RangeLines, A: a, B: b} }

var all = core.HookRange{Mode: core.RangeAll}

func TestParseBashRead(t *testing.T) {
	for _, tc := range []struct {
		cmd  string
		want bashRead
	}{
		{"cat main.go", bashRead{File: "main.go", Range: all}},
		{"cat -n main.go", bashRead{File: "main.go", Range: all}},
		{"cat 'a b.go'", bashRead{File: "a b.go", Range: all}},
		{`cat "a b.go"`, bashRead{File: "a b.go", Range: all}},
		{`cat a\ b.go`, bashRead{File: "a b.go", Range: all}},
		{"cat main.go | head -5", bashRead{File: "main.go", Range: all}},
		{"cat main.go 2>/dev/null", bashRead{File: "main.go", Range: all}},
		{"cat main.go 2>&1", bashRead{File: "main.go", Range: all}},
		{"cat main.go && echo done", bashRead{File: "main.go", Range: all}},
		{"nl -ba main.go", bashRead{File: "main.go", Range: all}},
		{"head main.go", bashRead{File: "main.go", Range: lines(1, 10)}},
		{"head -n 20 main.go", bashRead{File: "main.go", Range: lines(1, 20)}},
		{"head -n20 main.go", bashRead{File: "main.go", Range: lines(1, 20)}},
		{"head -5 main.go", bashRead{File: "main.go", Range: lines(1, 5)}},
		{"tail main.go", bashRead{File: "main.go", Range: core.HookRange{Mode: core.RangeTail, A: 10}}},
		{"tail -n 3 main.go", bashRead{File: "main.go", Range: core.HookRange{Mode: core.RangeTail, A: 3}}},
		{"tail -n +40 main.go", bashRead{File: "main.go", Range: lines(40, 0)}},
		{"sed -n '10,20p' main.go", bashRead{File: "main.go", Range: lines(10, 20)}},
		{"sed -n '7p' main.go", bashRead{File: "main.go", Range: lines(7, 7)}},
		{"sed -n '5,$p' main.go", bashRead{File: "main.go", Range: lines(5, 0)}},
		{"grep -n foo main.go", bashRead{File: "main.go", Range: all, Numbers: true}},
		{"grep -rn foo main.go", bashRead{}},
		{"grep -in foo main.go", bashRead{File: "main.go", Range: all, Numbers: true}},
		{"grep -e foo -n main.go", bashRead{File: "main.go", Range: all, Numbers: true}},
		{"grep --line-number foo main.go", bashRead{File: "main.go", Range: all, Numbers: true}},
		// not recorded
		{"grep foo main.go", bashRead{}},
		{"grep -n foo a.go b.go", bashRead{}},
		{"grep -nr foo .", bashRead{}},
		{"grep -nl foo main.go", bashRead{}},
		{"grep -nc foo main.go", bashRead{}},
		{"cat a.go b.go", bashRead{}},
		{"cat", bashRead{}},
		{"cat -", bashRead{}},
		{"cat $(ls)", bashRead{}},
		{"cat `ls`", bashRead{}},
		{`cat "$(ls)"`, bashRead{}},
		{"cat main.go > out.txt", bashRead{}},
		{"cat main.go >> out.txt", bashRead{}},
		{"cat main.go &> out.txt", bashRead{}},
		{"cat > out.txt", bashRead{}},
		{"cat >out.txt", bashRead{}},
		{"head -3 >> log.txt", bashRead{}},
		{"cat < main.go", bashRead{}},
		{"tail -f main.log", bashRead{}},
		{"tail -F main.log", bashRead{}},
		{"tail -c 10 main.go", bashRead{}},
		{"head -c 10 main.go", bashRead{}},
		{"sed -i 's/a/b/' main.go", bashRead{}},
		{"sed -i.bak 's/a/b/' main.go", bashRead{}},
		{"sed -n '10,20p' -i main.go", bashRead{}},
		{"sed 's/a/b/' main.go", bashRead{}},
		{"sed -n '20,10p' main.go", bashRead{}},
		{"sed -n 'p' main.go", bashRead{}},
		{"echo hello", bashRead{}},
		{"cd src && cat main.go", bashRead{}},
		{"ls | cat main.go", bashRead{}},
		{"cat 'unterminated", bashRead{}},
		{"", bashRead{}},
	} {
		got, ok := parseBashRead(tc.cmd)
		want := tc.want
		if (want.File != "") != ok || (ok && !reflect.DeepEqual(got, want)) {
			t.Errorf("%q: got %+v ok=%v, want %+v", tc.cmd, got, ok, want)
		}
	}
}

func TestNumberedLines(t *testing.T) {
	out := "12:func a() {\n13-\tx := 1\n20:func b() {\nnot a line\n"
	if got, want := numberedLines(out, "main.go"), map[string][]int{"main.go": {12, 13, 20}}; !reflect.DeepEqual(got, want) {
		t.Errorf("one file: %v", got)
	}
	multi := "a.go:3:x\nb/c.go:10:y\nb/c.go-11-z\na.go:4:w\n/abs/d.go:1:q\n"
	want := map[string][]int{"a.go": {3, 4}, "b/c.go": {10, 11}, "/abs/d.go": {1}}
	if got := numberedLines(multi, ""); !reflect.DeepEqual(got, want) {
		t.Errorf("several files: %v", got)
	}
	if got := numberedLines("12:text\n", ""); len(got) != 0 {
		t.Errorf("a line without a file name when the file is not known: %v", got)
	}
}

func TestRunsOf(t *testing.T) {
	for _, tc := range []struct {
		in   []int
		want [][2]int
	}{
		{nil, nil},
		{[]int{5}, [][2]int{{5, 5}}},
		{[]int{1, 2, 3, 7, 8, 12}, [][2]int{{1, 3}, {7, 8}, {12, 12}}},
		{[]int{4, 4, 5}, [][2]int{{4, 5}}},
	} {
		if got := runsOf(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%v: %v, want %v", tc.in, got, tc.want)
		}
	}
}
