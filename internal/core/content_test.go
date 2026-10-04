package core

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/amisonnet8/srwr/internal/tape"
)

func TestSelectWithExpect(t *testing.T) {
	const file = "a\nb\nc\nb\nc\nd\n"
	tests := []struct {
		name       string
		in         SelectInput
		code       string // "" is a success
		start, end int
		lines      []string
		in_msg     string // a part of the message of a failure
	}{
		{"line numbers only", SelectInput{StartLine: 2, EndLine: 3}, "", 2, 3, []string{"b", "c"}, ""},
		{"line numbers and the same lines", SelectInput{StartLine: 2, EndLine: 3, Expect: ptr("b\nc")}, "", 2, 3, []string{"b", "c"}, ""},
		{"a last line break is not a line", SelectInput{StartLine: 2, EndLine: 3, Expect: ptr("b\nc\n")}, "", 2, 3, []string{"b", "c"}, ""},
		{"an empty range and an empty expect", SelectInput{StartLine: 3, EndLine: 2, Expect: ptr("")}, "", 3, 2, []string{}, ""},
		{"an empty range and some lines", SelectInput{StartLine: 3, EndLine: 2, Expect: ptr("c")}, CodeContentMismatch, 0, 0, nil, "are not the lines of expect"},
		{"other lines, which are elsewhere", SelectInput{StartLine: 1, EndLine: 2, Expect: ptr("b\nc")}, CodeContentMismatch, 0, 0, nil, "at lines 2, 4"},
		{"other lines, which are in one place", SelectInput{StartLine: 1, EndLine: 1, Expect: ptr("d")}, CodeContentMismatch, 0, 0, nil, "at line 6"},
		{"other lines, which are nowhere", SelectInput{StartLine: 1, EndLine: 1, Expect: ptr("zzz")}, CodeContentMismatch, 0, 0, nil, "not in the file"},
		{"whitespace counts", SelectInput{StartLine: 1, EndLine: 1, Expect: ptr("a ")}, CodeContentMismatch, 0, 0, nil, ""},
		{"too many lines", SelectInput{StartLine: 5, EndLine: 5, Expect: ptr("c\nd")}, CodeContentMismatch, 0, 0, nil, "at line 5"},
		{"a range outside the file is still invalid_range", SelectInput{StartLine: 9, EndLine: 9, Expect: ptr("a")}, CodeInvalidRange, 0, 0, nil, ""},
		{"found in one place", SelectInput{Locate: true, Expect: ptr("d")}, "", 6, 6, []string{"d"}, ""},
		{"found at the start", SelectInput{Locate: true, Expect: ptr("a\nb")}, "", 1, 2, []string{"a", "b"}, ""},
		{"found at the end", SelectInput{Locate: true, Expect: ptr("c\nd\n")}, "", 5, 6, []string{"c", "d"}, ""},
		{"the whole file", SelectInput{Locate: true, Expect: ptr(strings.TrimSuffix(file, "\n"))}, "", 1, 6, []string{"a", "b", "c", "b", "c", "d"}, ""},
		{"found nowhere", SelectInput{Locate: true, Expect: ptr("z")}, CodeContentNotFound, 0, 0, nil, "(6 lines)"},
		{"an empty expect cannot find a range", SelectInput{Locate: true, Expect: ptr("")}, CodeInvalidInput, 0, 0, nil, "at least one line"},
		{"found in two places", SelectInput{Locate: true, Expect: ptr("b\nc")}, CodeContentAmbiguous, 0, 0, nil, "lines 2, 4"},
		{"one line in two places", SelectInput{Locate: true, Expect: ptr("c")}, CodeContentAmbiguous, 0, 0, nil, "in 2 places"},
		{"no expect to find the range with", SelectInput{Locate: true}, CodeInvalidInput, 0, 0, nil, ""},
		{"a CR", SelectInput{Locate: true, Expect: ptr("a\r")}, CodeInvalidInput, 0, 0, nil, "CR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			e.write("f.txt", file)
			in := tt.in
			in.File, in.Why = "f.txt", "確かめる"
			res, err := e.c.Select(in)
			if tt.code == "" {
				if err != nil {
					t.Fatalf("select: %v", err)
				}
				if res.StartLine != tt.start || res.EndLine != tt.end || !slices.Equal(res.Lines, tt.lines) {
					t.Errorf("range %d..%d %q, want %d..%d %q", res.StartLine, res.EndLine, res.Lines, tt.start, tt.end, tt.lines)
				}
				return
			}
			got := wantErr(t, err, tt.code)
			if !strings.Contains(got.Message, tt.in_msg) {
				t.Errorf("message %q lacks %q", got.Message, tt.in_msg)
			}
			if kinds := e.kinds(); slices.Contains(kinds, "select") {
				t.Errorf("a refused select is on the tape as a select: %v", kinds)
			}
			if e.read("f.txt") != file {
				t.Error("the file changed")
			}
		})
	}
}

func TestOverlappingPlacesCount(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "x\nx\nx\n")
	_, err := e.c.Select(SelectInput{File: "f.txt", Why: "w", Locate: true, Expect: ptr("x\nx")})
	if got := wantErr(t, err, CodeContentAmbiguous); !strings.Contains(got.Message, "lines 1, 2") {
		t.Errorf("message = %q: the two places overlap, and both count", got.Message)
	}
}

func TestContentMismatchSaysWhatIsThere(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "a\nb\nc\n")
	_, err := e.c.Select(SelectInput{File: "f.txt", StartLine: 1, EndLine: 2, Why: "w", Expect: ptr("b\nc")})
	got := wantErr(t, err, CodeContentMismatch)
	if lines, ok := got.Actual.([]string); !ok || !slices.Equal(lines, []string{"a", "b"}) {
		t.Errorf("actual = %#v, want the content of the range", got.Actual)
	}
	if !strings.Contains(got.Message, "line 2") {
		t.Errorf("message %q does not say where the lines are", got.Message)
	}
}

func TestPlacesAreCut(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", strings.Repeat("x\n", 30))
	_, err := e.c.Select(SelectInput{File: "f.txt", Why: "w", Locate: true, Expect: ptr("x")})
	got := wantErr(t, err, CodeContentAmbiguous)
	if !strings.Contains(got.Message, "in 30 places") || !strings.Contains(got.Message, "10 and more") || strings.Contains(got.Message, "11,") {
		t.Errorf("message = %q: it must say how many, and name only 10 places", got.Message)
	}
	_, err = e.c.Select(SelectInput{File: "f.txt", StartLine: 1, EndLine: 1, Why: "w", Expect: ptr("y\n")})
	if got := wantErr(t, err, CodeContentMismatch); !strings.Contains(got.Message, "not in the file") {
		t.Errorf("message = %q", got.Message)
	}
	e.write("g.txt", strings.Repeat("x\n", 8)+"y\n")
	_, err = e.c.Select(SelectInput{File: "g.txt", StartLine: 9, EndLine: 9, Why: "w", Expect: ptr("x")})
	if got := wantErr(t, err, CodeContentMismatch); !strings.Contains(got.Message, "5 and more") || strings.Contains(got.Message, "6,") {
		t.Errorf("message = %q: it must name only 5 places", got.Message)
	}
}

func TestExpectIsNotOnTheTape(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "a\nb\nc\n")
	const secretLine = "hunter2-not-in-the-file"
	e.c.Select(SelectInput{File: "f.txt", StartLine: 1, EndLine: 1, Why: "w", Expect: ptr(secretLine)})               //nolint:errcheck // the failure is what is checked
	e.c.Select(SelectInput{File: "f.txt", Locate: true, Why: "w", Expect: ptr(secretLine)})                           //nolint:errcheck // same
	e.c.Select(SelectInput{File: "f.txt", Locate: true, Why: "w", Expect: ptr("b")})                                  //nolint:errcheck // a success
	e.c.Select(SelectInput{File: "f.txt", StartLine: 1, EndLine: 3, Why: "w", Expect: ptr("a\nb\nc\n" + secretLine)}) //nolint:errcheck // a failure

	for _, ev := range e.events() {
		b, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), secretLine) {
			t.Errorf("expect is on the tape: %s", b)
		}
	}
	fails := e.failures()
	if len(fails) != 3 {
		t.Fatalf("%d failures, want 3: %+v", len(fails), fails)
	}
	if fails[0].StartLine == nil || *fails[0].StartLine != 1 {
		t.Errorf("a failure with line numbers lost them: %+v", fails[0])
	}
	if fails[1].StartLine != nil || fails[1].EndLine != nil || fails[1].Code != CodeContentNotFound {
		t.Errorf("a failure of a search has line numbers, or the wrong code: %+v", fails[1])
	}
	var picked *tape.Event
	for _, ev := range e.events() {
		if ev.Type == tape.TypeSelect {
			picked = &ev
		}
	}
	if picked == nil || picked.StartLine != 2 || picked.EndLine != 2 || picked.Selection == nil {
		t.Errorf("the select that found its range is on the tape as %+v", picked)
	}
}

func TestAFoundRangeCanBeReplaced(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "a\nb\nc\n")
	s, err := e.c.Select(SelectInput{File: "f.txt", Locate: true, Why: "w", Expect: ptr("b")})
	if err != nil {
		t.Fatal(err)
	}
	e.rep(e.c, s.Selection, "B")
	if got := e.read("f.txt"); got != "a\nB\nc\n" {
		t.Errorf("file = %q", got)
	}
	e.checkTape()
}
