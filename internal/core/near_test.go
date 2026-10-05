package core

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestNormLine(t *testing.T) {
	for in, want := range map[string]string{
		"":            "",
		"a":           "a",
		"\tif x {":    " if x {",
		"    if x {":  " if x {",
		"a \t b":      "a b",
		"a  \t":       "a",
		"  \t ":       "",
		"\t\treturn ": " return",
	} {
		if got := normLine(in); got != want {
			t.Errorf("normLine(%q) = %q, want %q", in, got, want)
		}
	}
}

const tabbed = "func f() {\n\tif x {\n\t\treturn 1\n\t}\n}\n"

func TestNearMatchesOfAnExpect(t *testing.T) {
	cases := []struct {
		name   string
		file   string
		lines  []int
		expect string
		code   string
		near   []NearMatch
		msg    string
	}{
		{
			name: "spaces for tabs, expect alone", file: tabbed, expect: "    if x {\n        return 1",
			code: CodeContentNotFound,
			near: []NearMatch{{StartLine: 2, EndLine: 3, Lines: []string{"\tif x {", "\t\treturn 1"}}},
			msg:  "Lines 2 to 3 differ from expect only in spaces or tabs: see nearMatches",
		},
		{
			name: "trailing spaces", file: "a\nb\n", expect: "b  ",
			code: CodeContentNotFound,
			near: []NearMatch{{StartLine: 2, EndLine: 2, Lines: []string{"b"}}},
			msg:  "Line 2 differs from expect only in spaces or tabs: see nearMatches",
		},
		{
			name: "two places", file: "\ta\nx\n\ta\n", expect: "  a",
			code: CodeContentNotFound,
			near: []NearMatch{{StartLine: 1, EndLine: 1, Lines: []string{"\ta"}}, {StartLine: 3, EndLine: 3, Lines: []string{"\ta"}}},
			msg:  "Lines 1, 3 differ from expect only in spaces or tabs: see nearMatches",
		},
		{
			name: "with line numbers, edit", file: tabbed, lines: []int{2, 2}, expect: "    if x {",
			code: CodeContentNotFound,
			near: []NearMatch{{StartLine: 2, EndLine: 2, Lines: []string{"\tif x {"}}},
			msg:  "Line 2 differs from expect only in spaces or tabs: see nearMatches",
		},
		{name: "a word differs: nothing near", file: tabbed, expect: "    if y {", code: CodeContentNotFound},
		{name: "a space inside a word is not whitespace folding", file: "ab\n", expect: "a b", code: CodeContentNotFound},
		{name: "six places, five are listed", file: strings.Repeat("\ta\n", 6), expect: "  a", code: CodeContentNotFound,
			msg: ""}, // checked below by count only
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.write("f.txt", tc.file)
			var lines []int
			if tc.lines != nil {
				lines = tc.lines
			}
			_, err := e.editAt("f.txt", lines, str(tc.expect), "X")
			wantCode(t, err, tc.code)
			if strings.HasPrefix(tc.name, "six") {
				if len(err.NearMatches) != maxNear {
					t.Errorf("near matches = %d, want %d", len(err.NearMatches), maxNear)
				}
				return
			}
			if !reflect.DeepEqual(err.NearMatches, tc.near) {
				t.Errorf("near = %+v, want %+v", err.NearMatches, tc.near)
			}
			if tc.msg != "" && !strings.HasSuffix(err.Message, tc.msg) {
				t.Errorf("message = %q, want it to end with %q", err.Message, tc.msg)
			}
			if tc.near == nil && strings.Contains(err.Message, "nearMatches") {
				t.Errorf("message = %q", err.Message)
			}
			if e.read("f.txt") != tc.file {
				t.Error("the file was changed")
			}
			// The lines are not on the tape, only the code and the message.
			e.noContentInFailures("return 1")
		})
	}
}

func TestNearMatchesOfALook(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", tabbed)
	// Located by expect alone.
	_, err := e.c.Look(LookInput{File: "f.txt", Locate: true, Expect: str("    if x {"), Why: "w"})
	wantCode(t, err, CodeContentNotFound)
	if len(err.NearMatches) != 1 || err.NearMatches[0].StartLine != 2 {
		t.Errorf("near = %+v", err.NearMatches)
	}
	// With line numbers whose lines are other, and the lines of expect are not in the file exactly.
	_, err = e.c.Look(LookInput{File: "f.txt", StartLine: 1, EndLine: 1, Expect: str("    if x {"), Why: "w"})
	wantCode(t, err, CodeContentMismatch)
	if len(err.NearMatches) != 1 || err.NearMatches[0].StartLine != 2 || !strings.Contains(err.Message, "see nearMatches") {
		t.Errorf("near = %+v, message %q", err.NearMatches, err.Message)
	}
	// The same lines exist exactly elsewhere: that is told by the message, and there is no near match.
	_, err = e.c.Look(LookInput{File: "f.txt", StartLine: 1, EndLine: 1, Expect: str("\tif x {"), Why: "w"})
	wantCode(t, err, CodeContentMismatch)
	if err.NearMatches != nil {
		t.Errorf("near = %+v", err.NearMatches)
	}
}

func TestNearMatchesOfAReplace(t *testing.T) {
	e := newEnv(t)
	e.write("a.go", "x\n\tfoo(\n\t\tbar)\ny\n\tfoo(\n\t\tbar)\n")
	e.write("b.go", "foo(\nbar)\n")
	// Two places in a.go differ in tabs; b.go has no indent at all, which is a difference of its own.
	_, err := e.sub([]string{"a.go", "b.go"}, "  foo(\n    bar)", "n(", 3)
	wantCode(t, err, CodeCountMismatch)
	want := []NearMatch{
		{File: "a.go", StartLine: 2, EndLine: 3, Lines: []string{"\tfoo(", "\t\tbar)"}},
		{File: "a.go", StartLine: 5, EndLine: 6, Lines: []string{"\tfoo(", "\t\tbar)"}},
	}
	if !reflect.DeepEqual(err.NearMatches, want) {
		t.Errorf("near = %+v, want %+v", err.NearMatches, want)
	}
	if !strings.Contains(err.Message, "a.go line 2") || strings.Contains(err.Message, "bar)") {
		t.Errorf("message = %q", err.Message)
	}
	e.noContentInFailures("bar)")

	// A place where old is exactly is not near; a part of a line is found too.
	e.write("c.go", "foo(1)\nfoo( 2)\n")
	_, err = e.sub([]string{"c.go"}, "foo(  2", "x", 2)
	wantCode(t, err, CodeCountMismatch)
	if len(err.NearMatches) != 1 || err.NearMatches[0].StartLine != 2 {
		t.Errorf("near = %+v", err.NearMatches)
	}
	_, err = e.sub([]string{"c.go"}, "foo(", "x", 3)
	wantCode(t, err, CodeCountMismatch)
	if err.NearMatches != nil {
		t.Errorf("exact places must not be listed as near: %+v", err.NearMatches)
	}

	// More places than asked for: nothing near is told.
	_, err = e.sub([]string{"a.go"}, "\tfoo(", "x", 1)
	if err == nil || err.NearMatches != nil {
		t.Errorf("err = %+v", err)
	}
}

// noContentInFailures fails if a failure on the tape holds any of the texts (a snapshot holds the file, rightly).
func (e *env) noContentInFailures(texts ...string) {
	e.t.Helper()
	for _, ev := range e.events() {
		if ev.Type != "failure" {
			continue
		}
		b, _ := json.Marshal(ev)
		for _, bad := range texts {
			if strings.Contains(string(b), bad) {
				e.t.Errorf("a failure on the tape holds %q: %s", bad, b)
			}
		}
	}
}
