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
