package core

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestRetryOfAFailedCall(t *testing.T) {
	const file = "a\n\tfoo(1)\nb\nx := foo(2)\n"
	cases := []struct {
		name string
		do   func(e *env) *Error
		want map[string]any
	}{
		{"look: spaces differ in one place", func(e *env) *Error {
			_, err := e.c.Look(LookInput{File: "f.txt", Expect: str("    foo(1)"), Locate: true, Why: "w"})
			return err
		}, map[string]any{"file": "f.txt", "startLine": 2, "endLine": 2, "expect": "\tfoo(1)"}},
		{"look: a part of one line", func(e *env) *Error {
			_, err := e.c.Look(LookInput{File: "f.txt", Expect: str("x := foo("), Locate: true, Why: "w"})
			return err
		}, map[string]any{"file": "f.txt", "startLine": 4, "endLine": 4, "expect": "x := foo(2)"}},
		{"look: the lines are elsewhere", func(e *env) *Error {
			_, err := e.c.Look(LookInput{File: "f.txt", StartLine: 1, EndLine: 1, Expect: str("b"), Why: "w"})
			return err
		}, map[string]any{"file": "f.txt", "startLine": 3, "endLine": 3, "expect": "b"}},
		{"look: a part of two lines: no retry", func(e *env) *Error {
			_, err := e.c.Look(LookInput{File: "f.txt", Expect: str("foo("), Locate: true, Why: "w"})
			return err
		}, nil},
		{"look: nothing like it", func(e *env) *Error {
			_, err := e.c.Look(LookInput{File: "f.txt", Expect: str("zzz"), Locate: true, Why: "w"})
			return err
		}, nil},
		{"edit: spaces differ, newText and insert are kept", func(e *env) *Error {
			_, err := e.c.Edit(EditInput{File: "f.txt", Expect: str("    foo(1)"), NewText: "new", Insert: "after", Why: "w"})
			return err
		}, map[string]any{"file": "f.txt", "startLine": 2, "endLine": 2, "expect": "\tfoo(1)", "newText": "new", "insert": "after"}},
		{"edit: a part of one line", func(e *env) *Error {
			_, err := e.c.Edit(EditInput{File: "f.txt", Expect: str("x := foo("), NewText: "y", Why: "w"})
			return err
		}, map[string]any{"file": "f.txt", "startLine": 4, "endLine": 4, "expect": "x := foo(2)", "newText": "y"}},
		{"edit: in an item of edits", func(e *env) *Error {
			_, err := e.c.Edits(EditsInput{Why: "w", Edits: []EditInput{{File: "f.txt", Expect: str("b"), NewText: "B"}, {File: "f.txt", Expect: str("x := foo("), NewText: "y"}}})
			return err
		}, map[string]any{"file": "f.txt", "startLine": 4, "endLine": 4, "expect": "x := foo(2)", "newText": "y"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.write("f.txt", file)
			err := tc.do(e)
			if err == nil {
				t.Fatal("want a failure")
			}
			if !reflect.DeepEqual(err.Retry, tc.want) {
				t.Errorf("retry = %v, want %v", err.Retry, tc.want)
			}
			// It is not on the tape.
			for _, ev := range e.events() {
				if ev.Type != "failure" {
					continue
				}
				if b, _ := json.Marshal(ev); tc.want != nil && strings.Contains(string(b), "retry") {
					t.Errorf("retry is on the tape: %s", b)
				}
			}
		})
	}
}

// The call a retry holds is the one that works.
func TestRetryWorks(t *testing.T) {
	e := newEnv(t)
	e.write("f.txt", "a\n\tfoo(1)\nb\n")
	_, err := e.c.Edit(EditInput{File: "f.txt", Expect: str("    foo(1)"), NewText: "\tfoo(2)", Why: "w"})
	r := err.Retry
	in := EditInput{File: r["file"].(string), HasLines: true, StartLine: r["startLine"].(int), EndLine: r["endLine"].(int),
		Expect: str(r["expect"].(string)), NewText: r["newText"].(string), Why: "w"}
	if _, err := e.c.Edit(in); err != nil {
		t.Fatalf("err = %+v", err)
	}
	if e.read("f.txt") != "a\n\tfoo(2)\nb\n" {
		t.Errorf("file = %q", e.read("f.txt"))
	}
}
