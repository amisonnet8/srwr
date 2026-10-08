package core

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/tape"
)

func TestSearchFindsLines(t *testing.T) {
	cases := []struct {
		name   string
		text   string
		search string
		lines  []int // the lines that match
		above  [][]string
		below  [][]string
	}{
		{name: "one line", text: "a\nfoo(1)\nb\nc\nd\n", search: "foo(", lines: []int{2},
			above: [][]string{{"a"}}, below: [][]string{{"b", "c"}}},
		{name: "several lines, the first and the last", text: "foo\nx\nfoo\n", search: "foo", lines: []int{1, 3},
			above: [][]string{{}, {"foo", "x"}}, below: [][]string{{"x", "foo"}, {}}},
		{name: "twice in a line is one match", text: "foofoo\n", search: "foo", lines: []int{1},
			above: [][]string{{}}, below: [][]string{{}}},
		{name: "case counts", text: "Foo\nfoo\n", search: "foo", lines: []int{2},
			above: [][]string{{"Foo"}}, below: [][]string{{}}},
		{name: "spaces count", text: "\tfoo\n", search: " foo", lines: nil},
		{name: "no match", text: "a\n", search: "zz", lines: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.write("f.txt", tc.text)
			res, err := e.c.Search(SearchInput{File: "f.txt", Search: tc.search, Why: "w"})
			if err != nil {
				t.Fatalf("err = %+v", err)
			}
			if res.Count != len(tc.lines) || len(res.Matches) != len(tc.lines) {
				t.Fatalf("count = %d, matches = %d, want %d", res.Count, len(res.Matches), len(tc.lines))
			}
			if res.Matches == nil {
				t.Error("Matches is nil: it must be an empty list")
			}
			looks := 0
			for _, ev := range e.events() {
				if ev.Type == tape.TypeLook {
					looks++
				}
			}
			if looks != len(tc.lines) {
				t.Errorf("looks on the tape = %d, want %d (none for no match)", looks, len(tc.lines))
			}
			for i, m := range res.Matches {
				if m.StartLine != tc.lines[i] || m.EndLine != tc.lines[i] {
					t.Errorf("match %d = %d..%d, want line %d", i, m.StartLine, m.EndLine, tc.lines[i])
				}
				if !slices.Equal(m.Above, tc.above[i]) || !slices.Equal(m.Below, tc.below[i]) {
					t.Errorf("match %d above = %v below = %v", i, m.Above, m.Below)
				}
			}
		})
	}
}

// The token of a match edits that one line, and the tape holds the look with the why and without the text searched for.
func TestSearchTokenEdits(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "a\nfoo\nb\nfoo\n")
	res, err := e.c.Search(SearchInput{File: "f.txt", Search: "foo", Why: "find foo"})
	if err != nil {
		t.Fatal(err)
	}
	// Edit the second match first: the first one is still right (srwr corrects the lines).
	if _, err := e.c.Edit(EditInput{Selection: res.Matches[1].Selection, NewText: "bar\nbar", Why: "w"}); err != nil {
		t.Fatalf("err = %+v", err)
	}
	if _, err := e.c.Edit(EditInput{Selection: res.Matches[0].Selection, NewText: "baz", Why: "w"}); err != nil {
		t.Fatalf("err = %+v", err)
	}
	if got := e.read("f.txt"); got != "a\nbaz\nb\nbar\nbar\n" {
		t.Errorf("file = %q", got)
	}
	for _, ev := range e.events() {
		if ev.Type == tape.TypeLook {
			if ev.Why == nil || *ev.Why != "find foo" || ev.Selection == nil || ev.Source != tape.SourceMCP {
				t.Errorf("look = %+v", ev)
			}
		}
	}
	e.checkTape()
}

func TestSearchLimit(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", strings.Repeat("foo\n", maxSearch+5))
	res, err := e.c.Search(SearchInput{File: "f.txt", Search: "foo", Why: "w"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != maxSearch+5 || len(res.Matches) != maxSearch {
		t.Errorf("count = %d, matches = %d", res.Count, len(res.Matches))
	}
	looks := 0
	for _, ev := range e.events() {
		if ev.Type == tape.TypeLook {
			looks++
		}
	}
	if looks != maxSearch {
		t.Errorf("looks on the tape = %d, want %d", looks, maxSearch)
	}
}

func TestSearchIsRefused(t *testing.T) {
	cases := []struct {
		name string
		in   SearchInput
		code string
	}{
		{"empty search", SearchInput{File: "f.txt", Search: "", Why: "w"}, CodeInvalidInput},
		{"LF in search", SearchInput{File: "f.txt", Search: "a\nb", Why: "w"}, CodeInvalidInput},
		{"CR in search", SearchInput{File: "f.txt", Search: "a\r", Why: "w"}, CodeInvalidInput},
		{"blank why", SearchInput{File: "f.txt", Search: "a", Why: " "}, CodeInvalidInput},
		{"no such file", SearchInput{File: "nope.txt", Search: "a", Why: "w"}, CodeFileNotFound},
		{"outside the workspace", SearchInput{File: "../f.txt", Search: "a", Why: "w"}, CodeInvalidRange},
		{"a file that is not recorded", SearchInput{File: ".env", Search: "a", Why: "w"}, CodeIgnoredFile},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.write("f.txt", "a\n")
			e.write(".env", "SECRET=a\n")
			_, err := e.c.Search(tc.in)
			if err == nil || err.Code != tc.code {
				t.Fatalf("err = %+v, want %s", err, tc.code)
			}
			// A failure is on the tape, without the text searched for.
			var fails []string
			for _, ev := range e.events() {
				if ev.Type == tape.TypeFailure {
					fails = append(fails, fmt.Sprintf("%+v", *ev.Failure))
				}
			}
			if len(fails) != 1 || strings.Contains(fails[0], "SECRET") {
				t.Errorf("failures = %v", fails)
			}
		})
	}
}

func TestSearchDirectory(t *testing.T) {
	e := newEnv(t)
	e.write("a.go", "x\nneedle 1\n")
	e.write("sub/b.go", "needle 2\nneedle 3\n")
	e.write("sub/none.go", "nothing\n")
	e.write(".env", "needle secret\n")
	e.write("bin.dat", "needle\x00\n")
	e.write("crlf.txt", "needle\r\n")

	res, err := e.c.Search(SearchInput{File: ".", Search: "needle", Why: "w"})
	if err != nil {
		t.Fatalf("err = %+v", err)
	}
	var got []string
	for _, m := range res.Matches {
		got = append(got, fmt.Sprintf("%s:%d", m.File, m.StartLine))
	}
	if want := "a.go:2 sub/b.go:1 sub/b.go:2"; strings.Join(got, " ") != want || res.Count != 3 {
		t.Errorf("matches = %v, count = %d, want %s", got, res.Count, want)
	}
	// Only the files with a match are on the tape: a snapshot and a look per match, and nothing for the other files.
	files := map[string]int{}
	for _, ev := range e.events() {
		if ev.File != "" {
			files[ev.File]++
		}
	}
	if len(files) != 2 || files["a.go"] != 2 || files["sub/b.go"] != 3 {
		t.Errorf("tape files = %v", files)
	}
	// A subdirectory, with a slash at the end.
	if res, err = e.c.Search(SearchInput{File: "sub/", Search: "needle", Why: "w"}); err != nil || res.Count != 2 {
		t.Errorf("sub: %+v %v", res, err)
	}
	// Nothing found writes nothing and is not an error.
	if res, err = e.c.Search(SearchInput{File: ".", Search: "zzz", Why: "w"}); err != nil || res.Count != 0 {
		t.Errorf("none: %+v %v", res, err)
	}
	// A match token works with edit.
	res, _ = e.c.Search(SearchInput{File: "sub", Search: "needle 3", Why: "w"})
	if _, err := e.c.Edit(EditInput{Selection: res.Matches[0].Selection, NewText: "done", Why: "w"}); err != nil {
		t.Errorf("edit: %+v", err)
	}
}

func TestSearchDirectoryIsCutAtTwentyMatches(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", strings.Repeat("needle\n", 15))
	e.write("b.txt", strings.Repeat("needle\n", 15))
	res, err := e.c.Search(SearchInput{File: ".", Search: "needle", Why: "w"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 30 || len(res.Matches) != maxSearch || res.Matches[19].File != "b.txt" {
		t.Errorf("count = %d, matches = %d", res.Count, len(res.Matches))
	}
}

func TestSearchDirectoryFailureHasNoSearchText(t *testing.T) {
	e := newEnv(t)
	_, err := e.c.Search(SearchInput{File: "../x", Search: "secret-needle", Why: "w"})
	wantCode(t, err, CodeInvalidRange)
	e.noContentInFailures("secret-needle")
}

func TestSearchDirectoryLeavesOutBigFiles(t *testing.T) {
	e := newEnv(t)
	e.write("big.txt", "needle\n"+strings.Repeat("x", maxSearchSize))
	e.write("small.txt", "needle\n")
	res, err := e.c.Search(SearchInput{File: ".", Search: "needle", Why: "w"})
	if err != nil || res.Count != 1 || res.Matches[0].File != "small.txt" {
		t.Errorf("%+v %v", res, err)
	}
}

func matchNames(res *SearchResult) string {
	var got []string
	for _, m := range res.Matches {
		got = append(got, fmt.Sprintf("%s:%d", m.File, m.StartLine))
	}
	return strings.Join(got, " ")
}

func TestSearchIncludeAndExclude(t *testing.T) {
	e := newEnv(t)
	e.write("a.go", "needle\n")
	e.write("a.md", "needle\n")
	e.write("internal/b.go", "needle\n")
	e.write("docs/c.md", "needle\n")
	e.write("docs/sub/d.md", "needle\n")
	cases := []struct {
		name             string
		include, exclude []string
		want             string
	}{
		{"none", nil, nil, "a.go:1 a.md:1 docs/c.md:1 docs/sub/d.md:1 internal/b.go:1"},
		{"extension at any depth", []string{"*.go"}, nil, "a.go:1 internal/b.go:1"},
		{"two includes", []string{"*.go", "docs/"}, nil, "a.go:1 docs/c.md:1 docs/sub/d.md:1 internal/b.go:1"},
		{"anchored with **", []string{"docs/**/*.md"}, nil, "docs/c.md:1 docs/sub/d.md:1"},
		{"exclude", nil, []string{"docs/"}, "a.go:1 a.md:1 internal/b.go:1"},
		{"include and exclude", []string{"*.md"}, []string{"docs/sub"}, "a.md:1 docs/c.md:1"},
		{"blank pattern is none", []string{""}, []string{""}, "a.go:1 a.md:1 docs/c.md:1 docs/sub/d.md:1 internal/b.go:1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := e.c.Search(SearchInput{File: ".", Search: "needle", Why: "w", Include: tc.include, Exclude: tc.exclude})
			if err != nil {
				t.Fatal(err)
			}
			if got := matchNames(res); got != tc.want {
				t.Errorf("matches = %s, want %s", got, tc.want)
			}
		})
	}
	// Files that do not pass are not on the tape.
	files := map[string]bool{}
	for _, ev := range e.events() {
		files[ev.File] = true
	}
	// (the earlier cases put every file there; a fresh workspace shows the filter)
	e2 := newEnv(t)
	e2.write("a.go", "needle\n")
	e2.write("a.md", "needle\n")
	if _, err := e2.c.Search(SearchInput{File: ".", Search: "needle", Why: "w", Include: []string{"*.go"}}); err != nil {
		t.Fatal(err)
	}
	for _, ev := range e2.events() {
		if ev.File == "a.md" {
			t.Errorf("a.md is on the tape: %+v", ev)
		}
	}
	// In one file, a filter that does not let it pass leaves no match.
	res, err := e2.c.Search(SearchInput{File: "a.md", Search: "needle", Why: "w", Include: []string{"*.go"}})
	if err != nil || res.Count != 0 {
		t.Errorf("one file: %+v %v", res, err)
	}
	_ = files
}

func TestSearchCutsLongLines(t *testing.T) {
	e := newEnv(t)
	long := strings.Repeat("a", 500) + "NEEDLE" + strings.Repeat("b", 500)
	e.write("f.txt", "short\n"+long+"\n"+strings.Repeat("あ", 300)+"\n")
	res, err := e.c.Search(SearchInput{File: "f.txt", Search: "NEEDLE", Why: "w"})
	if err != nil || len(res.Matches) != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	m := res.Matches[0]
	want := "…" + strings.Repeat("a", cutAround) + "NEEDLE" + strings.Repeat("b", cutAround) + "…"
	if m.Lines[0] != want || !m.Cut {
		t.Errorf("line = %q cut = %v", m.Lines[0], m.Cut)
	}
	if got := m.Below[0]; len([]rune(got)) != maxLineLen+1 || !strings.HasSuffix(got, "…") {
		t.Errorf("below not cut to %d characters: %d runes", maxLineLen, len([]rune(got)))
	}
	// The token and the tape are of the whole line.
	if _, err := e.c.Edit(EditInput{Selection: m.Selection, NewText: "x", Why: "w"}); err != nil {
		t.Errorf("edit: %+v", err)
	}
	for _, ev := range e.events() {
		if ev.Type == tape.TypeEdit && ev.OldText != long {
			t.Errorf("tape old text = %d chars, want the whole line", len(ev.OldText))
		}
	}
	// A short line is not cut (the line below it is, and cut says so).
	res, _ = e.c.Search(SearchInput{File: "f.txt", Search: "short", Why: "w"})
	if res.Matches[0].Lines[0] != "short" || !res.Matches[0].Cut {
		t.Errorf("short match = %+v", res.Matches[0])
	}
}

func TestSearchOffsetAndByFile(t *testing.T) {
	e := newEnv(t)
	e.write("a.txt", strings.Repeat("needle\n", 15))
	e.write("b.txt", strings.Repeat("needle\n", 12))
	e.write("c.txt", "needle\n")
	res, err := e.c.Search(SearchInput{File: ".", Search: "needle", Why: "w"})
	if err != nil || res.Count != 28 || len(res.Matches) != 20 {
		t.Fatalf("%+v %v", res, err)
	}
	if len(res.ByFile) != 3 || res.ByFile[0] != (FileCount{"a.txt", 15}) || res.ByFile[2] != (FileCount{"c.txt", 1}) {
		t.Errorf("byFile = %+v", res.ByFile)
	}
	before := 0
	for _, ev := range e.events() {
		if ev.Type == tape.TypeLook {
			before++
		}
	}
	res, err = e.c.Search(SearchInput{File: ".", Search: "needle", Why: "w", Offset: 20})
	if err != nil || res.Count != 28 || len(res.Matches) != 8 || res.ByFile != nil {
		t.Fatalf("%+v %v", res, err)
	}
	if got := matchNames(res); got != "b.txt:6 b.txt:7 b.txt:8 b.txt:9 b.txt:10 b.txt:11 b.txt:12 c.txt:1" {
		t.Errorf("second page = %s", got)
	}
	looks := 0
	for _, ev := range e.events() {
		if ev.Type == tape.TypeLook {
			looks++
		}
	}
	if looks-before != 8 {
		t.Errorf("the second page put %d looks on the tape, want 8", looks-before)
	}
	// Past the end: nothing.
	if res, _ = e.c.Search(SearchInput{File: ".", Search: "needle", Why: "w", Offset: 100}); res.Count != 28 || len(res.Matches) != 0 {
		t.Errorf("past the end: %+v", res)
	}
	_, err = e.c.Search(SearchInput{File: ".", Search: "needle", Why: "w", Offset: -1})
	wantCode(t, err, CodeInvalidInput)
}
